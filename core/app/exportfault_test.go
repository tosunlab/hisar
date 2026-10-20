package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingExportFile struct {
	*os.File
	failure string
	err     error
}

func (file *failingExportFile) Write(content []byte) (int, error) {
	if file.failure == "write" {
		return 0, file.err
	}
	return file.File.Write(content)
}
func (file *failingExportFile) Sync() error {
	if file.failure == "sync" {
		return file.err
	}
	return file.File.Sync()
}
func (file *failingExportFile) Close() error {
	err := file.File.Close()
	if file.failure == "close" {
		return file.err
	}
	return err
}

func TestExportRemovesFailedWriteSyncOrClose(t *testing.T) {
	for _, failure := range []string{"write", "sync", "close"} {
		t.Run(failure, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export.json")
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + failure + " failure")
			if err := finishNewExportFile(path, []byte("private export"), &failingExportFile{File: file, failure: failure, err: injected}); !errors.Is(err, injected) {
				t.Fatalf("write error = %v", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed export remains: %v", err)
			}
		})
	}
}
