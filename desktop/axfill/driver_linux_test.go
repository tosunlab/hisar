package axfill

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Chromium ignores a bubble's presses for its double-click interval after it appears and after each press, so the
// driver waits that long before each press, and a press it cannot wait for in time is not made.
func TestPromptPressesWaitForChromiumsInterval(t *testing.T) {
	times := &promptTimes{}
	shown := time.Now()
	times.noteShown(7, shown)
	if err := times.waitUntilPressable(context.Background(), 7); err != nil || time.Since(shown) < pressInterval {
		t.Fatalf("first press after %v, %v; want %v after the prompt showed", time.Since(shown), err, pressInterval)
	}
	pressed := time.Now()
	times.notePressed(7, pressed)
	ctx, cancel := context.WithTimeout(context.Background(), pressInterval/2)
	defer cancel()
	if err := times.waitUntilPressable(ctx, 7); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a second press within the interval: %v", err)
	}
	times.noteShown(7, time.Now())
	if err := times.waitUntilPressable(context.Background(), 7); err != nil || time.Since(pressed) < pressInterval {
		t.Errorf("second press after %v, %v; listing the prompt again must not move its first sight", time.Since(pressed), err)
	}
}
