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

func TestGetScreenSaverUnit(t *testing.T) {
	if unit := getScreenSaverUnit(":1.20", ":1.20"); unit != time.Millisecond {
		t.Errorf("KDE's screen locker answers milliseconds, got %v", unit)
	}
	for _, owners := range [][2]string{{":1.20", ""}, {":1.20", ":1.31"}, {"", ""}} {
		if unit := getScreenSaverUnit(owners[0], owners[1]); unit != time.Second {
			t.Errorf("owners %q: want seconds, got %v", owners, unit)
		}
	}
}
