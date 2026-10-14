package localsocket

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"
)

// ListenUnixSocket listens on path, readable only by the user, after removing a stale socket left there.
func ListenUnixSocket(path string) (net.Listener, error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	if err := removeStaleSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func removeStaleSocket(path string) error {
	fileInfo, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fileInfo.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("localsocket: %s exists and is not a socket", path)
	}
	if connection, err := net.DialTimeout("unix", path, time.Second); err == nil {
		connection.Close()
		return fmt.Errorf("localsocket: another process is listening on %s", path)
	}
	return os.Remove(path)
}
