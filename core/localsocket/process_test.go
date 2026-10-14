//go:build darwin || linux

package localsocket

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessHelpers(t *testing.T) {
	parentPID, err := GetParentPID(os.Getpid())
	if err != nil || parentPID != os.Getppid() {
		t.Errorf("parent of the test process = %d (%v), want %d", parentPID, err, os.Getppid())
	}
	path, err := GetProcessPath(os.Getpid())
	executable, _ := os.Executable()
	if err != nil || filepath.Base(path) != filepath.Base(executable) {
		t.Errorf("path of the test process = %q (%v), want %q", path, err, executable)
	}
	if _, err := GetParentPID(1 << 30); err == nil {
		t.Error("a missing process has a parent")
	}
}
