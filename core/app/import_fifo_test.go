//go:build unix

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A named pipe has no size to check beforehand, so the limit holds while reading.
func TestReadImportFileLimitsAPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.csv")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	go func() {
		writer, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer writer.Close()
		writer.Write(bytes.Repeat([]byte("a"), maxImportFileSize+1<<20))
	}()
	content, err := readImportFile(path)
	if err == nil || !strings.Contains(err.Error(), "larger than 100 MB") || content != nil {
		t.Errorf("read %d bytes, err = %v", len(content), err)
	}
}
