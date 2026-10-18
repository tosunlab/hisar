package systemidle

import (
	"testing"
	"time"
)

func TestReadIdleValue(t *testing.T) {
	if idle, isRead := readIdleValue([]any{uint64(90_500)}, time.Millisecond); !isRead || idle != 90500*time.Millisecond {
		t.Errorf("Mutter's milliseconds: %v %v", idle, isRead)
	}
	if idle, isRead := readIdleValue([]any{uint32(42)}, time.Second); !isRead || idle != 42*time.Second {
		t.Errorf("the screensaver's seconds: %v %v", idle, isRead)
	}
	for _, body := range [][]any{nil, {"42"}, {uint32(1), uint32(2)}} {
		if _, isRead := readIdleValue(body, time.Second); isRead {
			t.Errorf("read %v", body)
		}
	}
}
