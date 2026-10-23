// Package boundedstate holds capacity-limited, expiring state. Callers serialize table operations.
package boundedstate

import (
	"container/heap"
	"crypto/sha256"
	"runtime"
	"sync/atomic"
	"time"
)

const shardCount = 64

// Budget limits the combined live entries of tables which share it.
type Budget struct {
	capacity int64
	used     atomic.Int64
}

func NewBudget(capacity int) *Budget { return &Budget{capacity: int64(capacity)} }
func (b *Budget) Used() int          { return int(b.used.Load()) }
func (b *Budget) acquire() bool {
	for {
		n := b.used.Load()
		if n >= b.capacity {
			return false
		}
		if b.used.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// Entry owns one value and exactly one expiry-heap node.
type Entry[V any] struct {
	Key     [32]byte
	Value   V
	Expires time.Time
	index   int
}
type queue[V any] []*Entry[V]

func (q queue[V]) Len() int           { return len(q) }
func (q queue[V]) Less(i, j int) bool { return q[i].Expires.Before(q[j].Expires) }
func (q queue[V]) Swap(i, j int)      { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *queue[V]) Push(v any)        { e := v.(*Entry[V]); e.index = len(*q); *q = append(*q, e) }
func (q *queue[V]) Pop() any {
	old := *q
	n := len(old) - 1
	e := old[n]
	old[n] = nil
	*q = old[:n]
	return e
}

type shard[V any] struct {
	entries   map[[32]byte]*Entry[V]
	highWater int
}

// Table uses fixed-size key hashes and small independently reclaimable maps.
type Table[V any] struct {
	shards        [shardCount]shard[V]
	expiry        queue[V]
	capacity      int
	shardCapacity int
	budget        *Budget
	permits       *atomic.Int64
}
type cleanupBudget struct {
	budget  *Budget
	permits *atomic.Int64
}

func New[V any](capacity int, budget *Budget) *Table[V] {
	t := &Table[V]{capacity: capacity, shardCapacity: max(32, capacity/32+1), budget: budget, permits: new(atomic.Int64)}
	if budget != nil {
		runtime.AddCleanup(t, func(c cleanupBudget) { c.budget.used.Add(-c.permits.Swap(0)) }, cleanupBudget{budget, t.permits})
	}
	return t
}
func Key(key string) [32]byte                  { return sha256.Sum256([]byte(key)) }
func (t *Table[V]) Len() int                   { return len(t.expiry) }
func (t *Table[V]) Get(key [32]byte) *Entry[V] { return t.shards[int(key[0])%shardCount].entries[key] }
func (t *Table[V]) Insert(key [32]byte, value V, expires time.Time) bool {
	defer runtime.KeepAlive(t)
	s := &t.shards[int(key[0])%shardCount]
	if t.Len() >= t.capacity || len(s.entries) >= t.shardCapacity || t.Get(key) != nil {
		return false
	}
	if t.budget != nil && !t.budget.acquire() {
		return false
	}
	if t.budget != nil {
		t.permits.Add(1)
	}
	if s.entries == nil {
		s.entries = make(map[[32]byte]*Entry[V])
	}
	e := &Entry[V]{Key: key, Value: value, Expires: expires}
	s.entries[key] = e
	s.highWater = max(s.highWater, len(s.entries))
	heap.Push(&t.expiry, e)
	return true
}
func (t *Table[V]) Update(e *Entry[V], expires time.Time) {
	e.Expires = expires
	heap.Fix(&t.expiry, e.index)
}

// Visit applies an explicit configuration change to live entries. The callback may update expiry, not membership.
func (t *Table[V]) Visit(update func(*Entry[V])) {
	for i := range t.shards {
		for _, entry := range t.shards[i].entries {
			update(entry)
		}
	}
}
func (t *Table[V]) Remove(e *Entry[V]) V {
	defer runtime.KeepAlive(t)
	heap.Remove(&t.expiry, e.index)
	s := &t.shards[int(e.Key[0])%shardCount]
	delete(s.entries, e.Key)
	if len(s.entries) == 0 {
		s.entries = nil
		s.highWater = 0
	} else if len(s.entries)*4 < s.highWater {
		entries := make(map[[32]byte]*Entry[V], len(s.entries))
		for k, v := range s.entries {
			entries[k] = v
		}
		s.entries = entries
		s.highWater = len(entries)
	}
	if len(t.expiry) == 0 {
		t.expiry = nil
	} else if cap(t.expiry) > 1024 && len(t.expiry)*4 < cap(t.expiry) {
		expiry := make(queue[V], len(t.expiry))
		copy(expiry, t.expiry)
		t.expiry = expiry
	}
	if t.budget != nil {
		t.budget.used.Add(-1)
		t.permits.Add(-1)
	}
	value := e.Value
	var zero V
	e.Value = zero
	return value
}

// Expire does at most limit removals. It never scans live entries.
func (t *Table[V]) Expire(now time.Time, limit int, release func(V)) {
	for range limit {
		if len(t.expiry) == 0 || t.expiry[0].Expires.After(now) {
			return
		}
		value := t.Remove(t.expiry[0])
		if release != nil {
			release(value)
		}
	}
}
