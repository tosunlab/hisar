package systemidle

import (
	"runtime"
	"testing"
	"time"
)

func TestGetIdleDuration(t *testing.T) {
	idleDuration, isAvailable := GetIdleDuration()
	isMacBuild := runtime.GOOS == "darwin" && isCgoBuild
	// Linux has the idle time only on a desktop session, which a CI container has not.
	if isAvailable != isMacBuild && runtime.GOOS != "linux" {
		t.Fatalf("available = %v on %s (cgo %v)", isAvailable, runtime.GOOS, isCgoBuild)
	}
	if isAvailable && (idleDuration < 0 || idleDuration > 100*365*24*time.Hour) {
		t.Errorf("idle for %v", idleDuration)
	}
	if !isAvailable && idleDuration != 0 {
		t.Errorf("unavailable but idle for %v", idleDuration)
	}
}
