package api

import (
	"testing"

	"github.com/tryhisar/hisar/server/internal/realtime"
)

func TestEventBrokerEndsAllStreamExceptTheKeptOne(t *testing.T) {
	broker := realtime.NewBroker()
	kept, _ := broker.Subscribe("ALICE1", []byte("kept"))
	ended, _ := broker.Subscribe("ALICE1", []byte("ended"))
	other, _ := broker.Subscribe("BOB111", []byte("bob"))
	broker.EndStreams("ALICE1", []byte("kept"))
	if broker.IsEnded(kept) || !broker.IsEnded(ended) || broker.IsEnded(other) {
		t.Errorf("ended: kept %v, ended %v, other account %v", broker.IsEnded(kept), broker.IsEnded(ended), broker.IsEnded(other))
	}
	broker.EndStreams("ALICE1", nil)
	if !broker.IsEnded(kept) {
		t.Error("ending every stream kept one")
	}
}

func TestEventBrokerEndsTheStreamsOfGivenSessions(t *testing.T) {
	broker := realtime.NewBroker()
	laptop, _ := broker.Subscribe("ALICE1", []byte("laptop"))
	phone, _ := broker.Subscribe("ALICE1", []byte("phone"))
	other, _ := broker.Subscribe("BOB111", []byte("laptop"))
	broker.EndSessionStreams("ALICE1", [][]byte{[]byte("laptop")})
	if !broker.IsEnded(laptop) || broker.IsEnded(phone) || broker.IsEnded(other) {
		t.Errorf("ended: laptop %v, phone %v, other account %v", broker.IsEnded(laptop), broker.IsEnded(phone), broker.IsEnded(other))
	}
	broker.EndSessionStreams("ALICE1", nil)
	if broker.IsEnded(phone) {
		t.Error("no session ended a stream")
	}
}
