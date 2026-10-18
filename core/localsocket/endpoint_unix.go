//go:build !windows

package localsocket

import (
	"net"
	"path/filepath"
	"time"
)

// GetDefaultPath is ~/.hisar/<name>.sock, in the 0700 directory that keeps other users out.
func GetDefaultPath(homeDirectory, name string) string {
	return filepath.Join(homeDirectory, ".hisar", name+".sock")
}

// Listen serves the endpoint at path, reachable only by the user.
func Listen(path string) (net.Listener, error) {
	return ListenUnixSocket(path)
}

// Dial connects to the endpoint at path.
func Dial(path string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", path, timeout)
}
