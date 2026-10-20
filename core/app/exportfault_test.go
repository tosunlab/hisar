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

type changingIdentityExportFile struct {
	*failingExportFile
	created os.FileInfo
	written bool
}

func (file *changingIdentityExportFile) Stat() (os.FileInfo, error) {
	if !file.written {
		return file.created, nil
	}
	return file.File.Stat()
}

func (file *changingIdentityExportFile) Write(content []byte) (int, error) {
	n, err := file.File.Write(content)
	file.written = true
	if file.failure == "write" {
		return n, file.err
	}
	return n, err
}

func TestExportIdentityChangingAfterFirstWrite(t *testing.T) {
	for _, failure := range []string{"", "write", "sync", "close"} {
		t.Run("failure="+failure, func(t *testing.T) {
			directory := t.TempDir()
			otherPath := filepath.Join(directory, "creation-identity")
			if err := os.WriteFile(otherPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			created, err := os.Stat(otherPath)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "export.json")
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + failure + " failure")
			writer := &changingIdentityExportFile{
				failingExportFile: &failingExportFile{File: file, failure: failure, err: injected},
				created:           created,
			}
			err = finishNewExportFile(path, []byte("private export"), writer)
			if failure == "" {
				if err != nil {
					t.Fatal(err)
				}
				if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
					t.Fatalf("export = %q, %v", content, err)
				}
			} else {
				if !errors.Is(err, injected) {
					t.Fatalf("save error = %v", err)
				}
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed export remains after its inode changed: %v", err)
				}
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("export descriptor remains open: %v", err)
			}
		})
	}
}
