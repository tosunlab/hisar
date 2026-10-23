package service

import (
	"context"
	"github.com/tryhisar/hisar/server/internal/boundedstate"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"golang.org/x/time/rate"
	"runtime"
	"testing"
	"time"
)

func TestPendingCapacityNeverEvictsALiveExchange(t *testing.T) {
	cache := newLoginCache(time.Minute, 0)
	cache.pendingRequests = boundedstate.New[*pendingLogin](2, nil)
	first := cache.put(&pendingLogin{accountID: "real", clientMAC: []byte{1, 2, 3}})
	second := cache.put(&pendingLogin{accountID: "reauth", clientMAC: []byte{4}, sessionTokenHash: []byte{5}})
	refused := &pendingLogin{accountID: "overflow", clientMAC: []byte{9}, recordHash: []byte{8}}
	if first == "" || second == "" || cache.put(refused) != "" {
		t.Fatal("combined pending capacity was not enforced")
	}
	for range 10000 {
		if cache.put(&pendingLogin{}) != "" {
			t.Fatal("capacity attack got a slot")
		}
	}
	if refused.clientMAC[0] != 0 || refused.recordHash[0] != 0 {
		t.Fatal("refused secret state was retained")
	}
	for _, id := range []string{first, second} {
		pending := cache.take(id)
		if pending == nil {
			t.Fatal("live exchange was evicted")
		}
		pending.erase()
	}
	if cache.pendingRequests.Len() != 0 || cache.put(&pendingLogin{}) == "" {
		t.Fatal("finished exchange did not release its slot")
	}
}

func TestExpiredAuthenticationLeavesMemoryWithoutTrafficAndStops(t *testing.T) {
	factory := ratelimit.NewFactory(0, 0)
	service := &Service{loginCache: newLoginCache(time.Millisecond, 0), Limiters: newLimiters(factory), Approvals: newApprovalRelay(factory.New(rate.Every(approvalRateEvery), ApprovalRateBurst))}
	pending := &pendingLogin{clientMAC: []byte{1, 2}, recordHash: []byte{3}, sessionTokenHash: []byte{4}}
	service.loginCache.put(pending)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { service.RunAuthenticationMaintenance(ctx); close(done) }()
	defer cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		service.loginCache.mutex.Lock()
		remaining := service.loginCache.pendingRequests.Len()
		service.loginCache.mutex.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired authentication remained without traffic")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pending.clientMAC[0] != 0 || pending.recordHash[0] != 0 || pending.sessionTokenHash[0] != 0 {
		t.Fatal("expired MAC/hash slices were not cleared")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not stop")
	}
}

func TestMeasuredPendingCapacityHeap(t *testing.T) {
	runtime.GC()
	var before, full, after runtime.MemStats
	runtime.ReadMemStats(&before)
	cache := newLoginCache(time.Minute, 0)
	accepted, refused := 0, 0
	for range maximumPendingLogins + 1000 {
		if cache.put(&pendingLogin{accountID: "abcdefghijklmnopqrstuvwxyz012345", clientMAC: make([]byte, 64), recordHash: make([]byte, 32), sessionTokenHash: make([]byte, 32)}) == "" {
			refused++
		} else {
			accepted++
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&full)
	cache.mutex.Lock()
	cache.pendingRequests.Expire(time.Now().Add(2*time.Minute), maximumPendingLogins, (*pendingLogin).erase)
	cache.mutex.Unlock()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(cache)
	if accepted != maximumPendingLogins || refused != 1000 || cache.pendingRequests.Len() != 0 {
		t.Fatalf("accepted=%d refused=%d retained=%d", accepted, refused, cache.pendingRequests.Len())
	}
	t.Logf("one-run pending retained-heap sample accepted=%d refused=%d before=%d full=%d reclaimed=%d bytes", accepted, refused, before.HeapAlloc, full.HeapAlloc, after.HeapAlloc)
}
