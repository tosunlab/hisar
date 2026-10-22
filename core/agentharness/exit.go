package agentharness

import (
	"context"
	"time"
)

const exitPollInterval = time.Second

// IsRunning reports whether the session's harness process instance still runs.
func IsRunning(session Session) bool {
	instance, err := ReadInstance(session.PID)
	return err == nil && !instance.IsExited && instance.StartTime == session.StartTime
}

// pollForExit checks the process every second, where the platform cannot report its exit.
func pollForExit(ctx context.Context, session Session) {
	ticker := time.NewTicker(exitPollInterval)
	defer ticker.Stop()
	for IsRunning(session) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
