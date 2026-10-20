package app

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExclusiveExportDoesNotReplaceLaterFile(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "same session"
		if stale {
			name = "stale session"
		}
		t.Run(name, func(t *testing.T) {
			service := createUnlockedTestService(t).service
			directory := t.TempDir()
			path := filepath.Join(directory, "export.json")
			service.mutex.Lock()
			epoch := service.sessionEpoch
			done := make(chan error, 1)
			go func() { done <- service.saveNewFile(path, []byte("private export"), epoch) }()
			// The old writer made an empty destination before taking the mutex. The new writer prepares only a temp.
			deadline := time.Now().Add(5 * time.Second)
			for {
				entries, err := os.ReadDir(directory)
				if err != nil {
					service.mutex.Unlock()
					t.Fatal(err)
				}
				if len(entries) != 0 {
					break
				}
				if time.Now().After(deadline) {
					service.mutex.Unlock()
					t.Fatal("the export did not prepare its file")
				}
				time.Sleep(time.Millisecond)
			}
			other := path + ".other"
			if err := os.WriteFile(other, []byte("another writer's file"), 0o600); err != nil {
				service.mutex.Unlock()
				t.Fatal(err)
			}
			if err := os.Rename(other, path); err != nil {
				service.mutex.Unlock()
				t.Fatal(err)
			}
			if stale {
				service.sessionEpoch++
			}
			service.mutex.Unlock()
			err := <-done
			if err == nil || (!stale && !errors.Is(err, errExportFileExists)) {
				t.Errorf("saveNewFile error = %v", err)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "another writer's file" {
				t.Errorf("another writer's file changed: %q, %v", content, err)
			}
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
				t.Errorf("temporary output remains: %v, %v", entries, err)
			}
		})
	}
}

type exportCopyReader func([]byte) (int, error)

func (read exportCopyReader) Read(content []byte) (int, error) { return read(content) }

func TestExportCopyFallbackKeepsAnotherWritersReplacement(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "copy succeeds"
		if fail {
			name = "copy fails"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export.json")
			injected := errors.New("injected copy failure")
			read := exportCopyReader(func(content []byte) (int, error) {
				other := path + ".other"
				if err := os.WriteFile(other, []byte("another writer's file"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(other, path); err != nil {
					t.Fatal(err)
				}
				if fail {
					return 0, injected
				}
				return copy(content, "private export"), io.EOF
			})
			err := copyNewExportFile(path, read)
			if fail && !errors.Is(err, injected) || !fail && !errors.Is(err, errExportFileExists) {
				t.Errorf("copy error = %v", err)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "another writer's file" {
				t.Errorf("another writer's file changed: %q, %v", content, err)
			}
		})
	}
}

func TestExportCopyFallbackCreatesOnlyANewPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.json")
	if err := copyNewExportFile(path, strings.NewReader("private export")); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
		t.Fatalf("copied export = %q, %v", content, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("copied export permissions = %v, %v", info, err)
	}
	if err := copyNewExportFile(path, strings.NewReader("replacement")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("copy over an existing file = %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
		t.Errorf("existing export changed: %q, %v", content, err)
	}
}

func TestExportCopyFallbackRemovesItsOwnFailedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.json")
	injected := errors.New("injected copy failure")
	err := copyNewExportFile(path, exportCopyReader(func(content []byte) (int, error) {
		return copy(content, "partial export"), injected
	}))
	if !errors.Is(err, injected) {
		t.Fatalf("copy error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("failed export remains: %v", err)
	}
}
