package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestDotenvRewriteRefusesChangedSource(t *testing.T) {
	for _, change := range []string{"edited", "replaced", "redirected", "permissions"} {
		t.Run(change, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, ".env")
			if err := os.WriteFile(path, []byte("TOKEN=original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, source, err := readDotenvSource(path)
			if err != nil {
				t.Fatal(err)
			}
			const changed = "TOKEN=changed\n"
			switch change {
			case "edited":
				err = os.WriteFile(path, []byte(changed), 0o600)
			case "replaced":
				replacement := filepath.Join(directory, "new.env")
				if err = os.WriteFile(replacement, []byte(changed), 0o600); err == nil {
					err = os.Rename(replacement, path)
				}
			case "permissions":
				err = os.Chmod(path, 0o644)
			case "redirected":
				target := filepath.Join(directory, "other.env")
				if err = os.WriteFile(target, []byte(changed), 0o600); err == nil {
					if err = os.Remove(path); err == nil {
						err = os.Symlink(target, path)
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := rewriteDotenvFile(path, "TOKEN=hisar://Personal/Environment/TOKEN\n", source); err == nil {
				t.Fatal("rewrote a changed source")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatalf("source changed: %q, %v", after, err)
			}
		})
	}
}

func TestDotenvRewriteKeepsAnUnchangedSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.env")
	path := filepath.Join(directory, ".env")
	if err := os.WriteFile(target, []byte("TOKEN=secret\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	_, source, err := readDotenvSource(path)
	if err != nil {
		t.Fatal(err)
	}
	const rendered = "TOKEN=hisar://Personal/Environment/TOKEN\n"
	if err := rewriteDotenvFile(path, rendered, source); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != rendered {
		t.Fatalf("target = %q, %v", content, err)
	}
	info, err = os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode changed: %v", err)
	}
}

type dotenvGuardBackend struct {
	retentionBackend
	path    string
	content string
	isSaved bool
}

func (access *dotenvGuardBackend) listItems(context.Context, string) ([]item.Item, error) {
	replacement := access.path + ".new"
	if err := os.WriteFile(replacement, []byte(access.content), 0o600); err != nil {
		return nil, err
	}
	return nil, os.Rename(replacement, access.path)
}

func (access *dotenvGuardBackend) saveItem(context.Context, string, *item.Item, ...string) error {
	access.isSaved = true
	return nil
}

func TestDotenvChecksBeforeStoringAndAllowsUnchangedAtomicSave(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged atomic save", true: "changed atomic save"}[changed], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte("TOKEN=original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			content := "TOKEN=original\n"
			if changed {
				content = "TOKEN=changed\n"
			}
			access := &dotenvGuardBackend{path: path, content: content}
			err := importDotenv(context.Background(), access, path, dotenvImportOption{vaultName: "Personal", isVaultChosen: true, itemTitle: "Environment", isConfirmed: true})
			if changed {
				if err == nil || access.isSaved {
					t.Fatalf("stored stale values: saved %v, error %v", access.isSaved, err)
				}
			} else if err != nil || !access.isSaved {
				t.Fatalf("refused unchanged atomic save: saved %v, error %v", access.isSaved, err)
			}
			after, readErr := os.ReadFile(path)
			want := "TOKEN=hisar://Personal/Environment/TOKEN\n"
			if changed {
				want = content
			}
			if readErr != nil || string(after) != want {
				t.Fatalf("source = %q, want %q, error %v", after, want, readErr)
			}
		})
	}
}
