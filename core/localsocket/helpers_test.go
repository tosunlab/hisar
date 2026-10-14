package localsocket

import (
	"os"
	"testing"
)

func createShortTempDirectory(t *testing.T) string {
	t.Helper()
	// Unix socket paths are limited to about 104 bytes, which t.TempDir can exceed on macOS.
	directory, err := os.MkdirTemp("", "hisar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return directory
}
