package realtime

import "testing"

func TestEventBrokerOverflowEndsStream(t *testing.T) {
	broker := NewBroker()
	subscriber, isSubscribed := broker.Subscribe("ABC123", nil)
	if !isSubscribed {
		t.Fatal("not subscribed")
	}
	for range SubscriberBufferSize + 1 {
		broker.Publish([]string{"ABC123", "OTHER1"}, Event{Name: "vaults-changed"})
	}
	if !broker.IsOverflowed(subscriber) || len(subscriber.Events) != SubscriberBufferSize {
		t.Errorf("overflowed = %v with %d queued", broker.IsOverflowed(subscriber), len(subscriber.Events))
	}
	broker.Unsubscribe("ABC123", subscriber)
	if broker.CountSubscribers("ABC123") != 0 || len(broker.subscribersByAccount) != 0 {
		t.Error("unsubscribe left state behind")
	}
}
