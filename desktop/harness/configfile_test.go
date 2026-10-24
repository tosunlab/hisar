package harness

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

var testConfigFile = configFile{name: "the agent's config.yaml", folderName: "the agent's folder"}

type fileState struct {
	link    string
	mode    fs.FileMode
	content string
}

func readFileState(t *testing.T, path string) fileState {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			t.Fatal(err)
		}
		return fileState{link: link}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fileState{mode: info.Mode(), content: string(content)}
}

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReplaceConfigKeepsAFileReplacedMeanwhile(t *testing.T) {
	const content = "model: x\n"
	for _, test := range []struct {
		name   string
		change func(t *testing.T, path, directory string)
	}{
		{"its mode changed", func(t *testing.T, path, _ string) { mustDo(t, os.Chmod(path, 0o400)) }},
		{"another file with the same content", func(t *testing.T, path, directory string) {
			other := filepath.Join(directory, "other.yaml")
			mustDo(t, os.WriteFile(other, []byte(content), 0o600))
			mustDo(t, os.Rename(other, path))
		}},
		{"a link to a file with the same content", func(t *testing.T, path, directory string) {
			target := filepath.Join(directory, "dotfile.yaml")
			mustDo(t, os.WriteFile(target, []byte(content), 0o600))
			mustDo(t, os.Remove(path))
			mustDo(t, os.Symlink(target, path))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "config.yaml")
			mustDo(t, os.WriteFile(path, []byte(content), 0o600))
			read, err := testConfigFile.read(path)
			mustDo(t, err)
			test.change(t, path, directory)
			want := readFileState(t, path)
			var changed configChangedError
			if _, err := testConfigFile.replace(read, []byte("model: changed\n")); !errors.As(err, &changed) {
				t.Fatalf("replace() error = %v; want configChangedError", err)
			}
			if got := readFileState(t, path); got != want {
				t.Fatalf("config.yaml = %+v; want the other program's %+v", got, want)
			}
		})
	}
}

func TestReplaceConfigKeepsALinkRetargetedMeanwhile(t *testing.T) {
	directory := t.TempDir()
	path, first, second := filepath.Join(directory, "config.yaml"), filepath.Join(directory, "first.yaml"), filepath.Join(directory, "second.yaml")
	for _, target := range []string{first, second} {
		mustDo(t, os.WriteFile(target, []byte("model: x\n"), 0o600))
	}
	mustDo(t, os.Symlink(first, path))
	read, err := testConfigFile.read(path)
	mustDo(t, err)
	mustDo(t, os.Remove(path))
	mustDo(t, os.Symlink(second, path))
	var changed configChangedError
	if _, err := testConfigFile.replace(read, []byte("model: changed\n")); !errors.As(err, &changed) {
		t.Fatalf("replace() error = %v; want configChangedError", err)
	}
	for _, target := range []string{first, second} {
		if got := readFileState(t, target); got.content != "model: x\n" {
			t.Fatalf("%s = %q; want it unchanged", filepath.Base(target), got.content)
		}
	}
	if got := readFileState(t, path); got.link != second {
		t.Fatalf("config.yaml = %+v; want the link to %s", got, second)
	}
}

func TestRestoreConfigPutsBackWhatHisarRead(t *testing.T) {
	for _, test := range []struct {
		name     string
		original *string
	}{
		{"a file Hisar changed", new("model: x\n")},
		{"a file Hisar created", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if test.original != nil {
				mustDo(t, os.WriteFile(path, []byte(*test.original), 0o640))
			}
			original, err := testConfigFile.read(path)
			mustDo(t, err)
			written, err := testConfigFile.replace(original, []byte("mcp_servers:\n  hisar: {}\n"))
			mustDo(t, err)
			if err := testConfigFile.restore(original, written); err != nil {
				t.Fatalf("restore() = %v", err)
			}
			_, err = os.Lstat(path)
			switch {
			case test.original == nil && !errors.Is(err, fs.ErrNotExist):
				t.Fatalf("config.yaml after restore: %v; want none", err)
			case test.original != nil && readFileState(t, path) != fileState{mode: 0o640, content: *test.original}:
				t.Fatalf("config.yaml after restore = %+v", readFileState(t, path))
			}
		})
	}
}

func TestRestoreConfigReportsAChangeItKept(t *testing.T) {
	for _, test := range []struct {
		name     string
		original *string
	}{
		{"a file Hisar changed", new("model: x\n")},
		{"a file Hisar created", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if test.original != nil {
				mustDo(t, os.WriteFile(path, []byte(*test.original), 0o600))
			}
			original, err := testConfigFile.read(path)
			mustDo(t, err)
			written, err := testConfigFile.replace(original, []byte("mcp_servers:\n  hisar: {}\n"))
			mustDo(t, err)
			concurrent := "mcp_servers:\n  hisar: {}\nmodel: user-change\n"
			mustDo(t, os.WriteFile(path, []byte(concurrent), 0o600))
			var changed configChangedError
			if err := testConfigFile.restore(original, written); !errors.As(err, &changed) {
				t.Fatalf("restore() = %v; want configChangedError", err)
			}
			if got := readFileState(t, path); got.content != concurrent {
				t.Fatalf("config.yaml = %q; want the other program's change", got.content)
			}
			refusal := errors.New("Hermes reads Hisar's server differently")
			want := "Hermes did not accept the server Hisar added (Hermes reads Hisar's server differently), and the agent's config.yaml changed after Hisar wrote it, so Hisar left it as it is. Remove the server named hisar from it if it is still there"
			if err := testConfigFile.rollBack("Hermes", refusal, original, written); err == nil || err.Error() != want || !errors.Is(err, refusal) {
				t.Fatalf("rollBack() = %v; want %q", err, want)
			}
		})
	}
}
