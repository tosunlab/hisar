package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
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

func TestExportCopyFallbackKeepsAnotherWritersReplacement(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "copy succeeds"
		if fail {
			name = "copy fails"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export.json")
			injected := errors.New("injected copy failure")
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			write := func() error {
				other := path + ".other"
				if err := os.WriteFile(other, []byte("another writer's file"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(other, path); err != nil {
					t.Fatal(err)
				}
				if fail {
					return injected
				}
				_, err := file.Write([]byte("private export"))
				return err
			}
			err = finishExportFile(path, file, write)
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
	if err := publishExportWithoutLinks(t, path, []byte("private export"), syscall.ENOTSUP); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
		t.Fatalf("copied export = %q, %v", content, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("copied export permissions = %v, %v", info, err)
	}
	if err := publishExportWithoutLinks(t, path, []byte("replacement"), syscall.ENOTSUP); !errors.Is(err, errExportFileExists) {
		t.Fatalf("copy over an existing file = %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
		t.Errorf("existing export changed: %q, %v", content, err)
	}
}

func TestExportCopyFallbackRemovesItsOwnFailedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.json")
	injected := errors.New("injected copy failure")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = finishExportFile(path, file, func() error {
		if _, err := file.Write([]byte("partial export")); err != nil {
			return err
		}
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatalf("copy error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("failed export remains: %v", err)
	}
}

func publishExportWithoutLinks(t *testing.T, path string, content []byte, linkErr error) error {
	t.Helper()
	service := createUnlockedTestService(t).service
	return service.saveSessionFile(path, content, service.sessionEpoch, func(temporaryPath, path string) error {
		return publishNewExportFile(temporaryPath, path, content, func(string, string) error {
			return &os.LinkError{Op: "link", Old: temporaryPath, New: path, Err: linkErr}
		})
	})
}

func TestExportFallbackUnsupportedLinkErrors(t *testing.T) {
	for _, linkErr := range []error{syscall.EPERM, syscall.ENOTSUP, syscall.EOPNOTSUPP, syscall.ENOSYS, syscall.EXDEV, errors.ErrUnsupported} {
		t.Run(linkErr.Error(), func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "export.json")
			if err := publishExportWithoutLinks(t, path, []byte("private export"), linkErr); err != nil {
				t.Fatal(err)
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != "private export" {
				t.Fatalf("export = %q, %v", content, err)
			}
			if entries, err := os.ReadDir(directory); err != nil || len(entries) != 1 {
				t.Fatalf("temporary output remains: %v, %v", entries, err)
			}
		})
	}
}

func TestExportLinkFailureLeavesNoOutput(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "export.json")
	if err := publishExportWithoutLinks(t, path, []byte("private export"), syscall.EIO); !errors.Is(err, syscall.EIO) {
		t.Fatalf("link failure = %v", err)
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
		t.Fatalf("failed export remains: %v, %v", entries, err)
	}
}
