package harness

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type configFile struct {
	name       string
	folderName string
}

type configChangedError struct {
	name string
}

func (err configChangedError) Error() string {
	return err.name + " changed while Hisar was connecting. Try again"
}

type configSnapshot struct {
	configPath string
	path       string
	link       string
	content    []byte
	mode       fs.FileMode
	info       fs.FileInfo
}

func (snapshot configSnapshot) isSame(other configSnapshot) bool {
	if snapshot.path != other.path || snapshot.link != other.link || (snapshot.info == nil) != (other.info == nil) {
		return false
	}
	return snapshot.info == nil || os.SameFile(snapshot.info, other.info) && snapshot.info.Mode() == other.info.Mode() &&
		bytes.Equal(snapshot.content, other.content)
}

// resolvePath is the file to change: configPath, or the file a configPath linked from a dotfiles folder points to, so
// the link stays, with the link itself. A link to nothing is refused: replacing it would cut the user's link, and
// creating the file it names would write into a folder that may no longer be where the user keeps their dotfiles.
func (file configFile) resolvePath(configPath string) (string, string, error) {
	info, err := os.Lstat(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return configPath, "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("Hisar cannot read %s: %w", file.name, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return configPath, "", nil
	}
	link, err := os.Readlink(configPath)
	if err != nil {
		return "", "", fmt.Errorf("Hisar cannot read %s: %w", file.name, err)
	}
	target, err := filepath.EvalSymlinks(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", fmt.Errorf("%s links to %s, which does not exist. Fix the link and try again", file.name, link)
	}
	if err != nil {
		return "", "", fmt.Errorf("Hisar cannot read %s: %w", file.name, err)
	}
	return target, link, nil
}

func (file configFile) read(configPath string) (configSnapshot, error) {
	path, link, err := file.resolvePath(configPath)
	if err != nil {
		return configSnapshot{}, err
	}
	snapshot := configSnapshot{configPath: configPath, path: path, link: link, mode: 0o600}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot read %s: %w", file.name, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot read %s: %w", file.name, err)
	}
	snapshot.content, snapshot.mode, snapshot.info = content, info.Mode().Perm(), info
	return snapshot, nil
}

// replace writes content to a temporary file beside the file and renames it over the file, so the agent never reads
// half a file, and returns the file it wrote. Right before the rename it reads the file again and writes nothing when
// it is not the file Hisar read: a changed link, another file in its place, another type, mode or content. The agents
// take no lock Hisar could share, so the check is optimistic: a change made between it and the rename is lost.
func (file configFile) replace(expected configSnapshot, content []byte) (configSnapshot, error) {
	directory := filepath.Dir(expected.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot create %s: %w", file.folderName, err)
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(expected.path)+".hisar-*")
	if err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot write %s: %w", file.name, err)
	}
	defer os.Remove(temporary.Name())
	_, err = temporary.Write(content)
	if err == nil {
		err = temporary.Chmod(expected.mode)
	}
	if err == nil {
		err = temporary.Sync()
	}
	var info fs.FileInfo
	if err == nil {
		info, err = temporary.Stat()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot write %s: %w", file.name, err)
	}
	current, err := file.read(expected.configPath)
	if err != nil {
		return configSnapshot{}, err
	}
	if !current.isSame(expected) {
		return configSnapshot{}, configChangedError{name: file.name}
	}
	if err := os.Rename(temporary.Name(), expected.path); err != nil {
		return configSnapshot{}, fmt.Errorf("Hisar cannot write %s: %w", file.name, err)
	}
	written := expected
	written.content, written.info = content, info
	return written, nil
}

// restore puts back the file Hisar read, or removes the one Hisar created, as long as the file is still the one
// Hisar wrote. Otherwise it is configChangedError and the file stays as the other program left it.
func (file configFile) restore(original, written configSnapshot) error {
	if original.info != nil {
		_, err := file.replace(written, original.content)
		return err
	}
	current, err := file.read(written.configPath)
	if err != nil {
		return err
	}
	if !current.isSame(written) {
		return configChangedError{name: file.name}
	}
	if err := os.Remove(written.path); err != nil {
		return fmt.Errorf("Hisar cannot write %s: %w", file.name, err)
	}
	return nil
}

func (file configFile) rollBack(agent string, refusal error, original, written configSnapshot) error {
	err := file.restore(original, written)
	var changed configChangedError
	switch {
	case err == nil:
		return fmt.Errorf("%s did not accept the server Hisar added, so Hisar took it out again: %w", agent, refusal)
	case errors.As(err, &changed):
		return fmt.Errorf("%s did not accept the server Hisar added (%w), and %s changed after Hisar wrote it, so Hisar left it as it is. Remove the server named hisar from it if it is still there",
			agent, refusal, file.name)
	}
	return fmt.Errorf("%s did not accept the server Hisar added (%w), and Hisar could not take it out again: %w", agent, refusal, err)
}
