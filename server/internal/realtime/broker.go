// Package realtime fans events out to the server-sent event streams of one server process, per account.
package realtime

import (
	"bytes"
	"slices"
	"sync"
)

// Limits of the streams of one account.
const (
	MaxStreamsPerAccount = 8
	SubscriberBufferSize = 32
)

type Event struct {
	Name string
	Data any
}

type Subscriber struct {
	TokenHash []byte
	Events    chan Event
	// isOverflowed is set when an event could not be queued; the stream then ends so the client resyncs.
	isOverflowed bool
	isEnded      bool
}

// Broker fans vault changes out to the event streams of one server process.
type Broker struct {
	mutex                sync.Mutex
	subscribersByAccount map[string]map[*Subscriber]struct{}
}

func NewBroker() *Broker {
	return &Broker{subscribersByAccount: map[string]map[*Subscriber]struct{}{}}
}

func (broker *Broker) Subscribe(accountID string, tokenHash []byte) (*Subscriber, bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	subscribers := broker.subscribersByAccount[accountID]
	if len(subscribers) >= MaxStreamsPerAccount {
		return nil, false
	}
	if subscribers == nil {
		subscribers = map[*Subscriber]struct{}{}
		broker.subscribersByAccount[accountID] = subscribers
	}
	subscriber := &Subscriber{TokenHash: tokenHash, Events: make(chan Event, SubscriberBufferSize)}
	subscribers[subscriber] = struct{}{}
	return subscriber, true
}

func (broker *Broker) Unsubscribe(accountID string, subscriber *Subscriber) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	delete(broker.subscribersByAccount[accountID], subscriber)
	if len(broker.subscribersByAccount[accountID]) == 0 {
		delete(broker.subscribersByAccount, accountID)
	}
}

func (broker *Broker) Publish(accountIDs []string, event Event) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for _, accountID := range accountIDs {
		for subscriber := range broker.subscribersByAccount[accountID] {
			select {
			case subscriber.Events <- event:
			default:
				subscriber.isOverflowed = true
			}
		}
	}
}

// EndStreams closes the account's streams except the one of keptTokenHash, without waiting for the next heartbeat.
func (broker *Broker) EndStreams(accountID string, keptTokenHash []byte) {
	broker.endStreamsWhere(accountID, func(subscriber *Subscriber) bool {
		return keptTokenHash == nil || !bytes.Equal(subscriber.TokenHash, keptTokenHash)
	})
}

// EndSessionStreams closes the account's streams of the sessions with endedTokenHashes.
func (broker *Broker) EndSessionStreams(accountID string, endedTokenHashes [][]byte) {
	broker.endStreamsWhere(accountID, func(subscriber *Subscriber) bool {
		return slices.ContainsFunc(endedTokenHashes, func(tokenHash []byte) bool { return bytes.Equal(subscriber.TokenHash, tokenHash) })
	})
}

func (broker *Broker) endStreamsWhere(accountID string, shouldEnd func(subscriber *Subscriber) bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for subscriber := range broker.subscribersByAccount[accountID] {
		if !shouldEnd(subscriber) {
			continue
		}
		subscriber.isEnded = true
		select {
		case subscriber.Events <- Event{}:
		default:
		}
	}
}

func (broker *Broker) IsEnded(subscriber *Subscriber) bool {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return subscriber.isEnded
}

func (broker *Broker) IsOverflowed(subscriber *Subscriber) bool {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return subscriber.isOverflowed
}

func (broker *Broker) CountSubscribers(accountID string) int {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return len(broker.subscribersByAccount[accountID])
}
