package localsocket

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const pipePrefix = `\\.\pipe\`

// Named pipes share one namespace on the machine, so the name carries the user's SID and both ends check the other's.
func GetDefaultPath(homeDirectory, name string) string {
	sid, err := getCurrentUserSID()
	if err != nil {
		return filepath.Join(homeDirectory, ".hisar", name+".sock")
	}
	return pipePrefix + "hisar-" + name + "-" + sid.String()
}

func isPipePath(path string) bool {
	return strings.HasPrefix(path, pipePrefix)
}

// Listen creates the pipe's first instance with a DACL for the user alone; a pipe someone else created first makes it fail.
// A path that is not a pipe is a Unix socket, which Windows 10 1803 and later support.
func Listen(path string) (net.Listener, error) {
	if !isPipePath(path) {
		return ListenUnixSocket(path)
	}
	sid, err := getCurrentUserSID()
	if err != nil {
		return nil, err
	}
	listener, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid.String() + ")"})
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_PIPE_BUSY) {
		return nil, fmt.Errorf("localsocket: another process already serves %s", path)
	}
	return listener, err
}

// Dial connects to the pipe and refuses one whose server runs as another user.
func Dial(path string, timeout time.Duration) (net.Conn, error) {
	if !isPipePath(path) {
		return net.DialTimeout("unix", path, timeout)
	}
	connection, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return nil, err
	}
	if err := checkPipeServer(connection); err != nil {
		connection.Close()
		return nil, err
	}
	return connection, nil
}

type handleConnection interface {
	Fd() uintptr
}

func getPipeHandle(connection net.Conn) (windows.Handle, error) {
	handle, ok := connection.(handleConnection)
	if !ok {
		return 0, errors.New("localsocket: connection is not a named pipe")
	}
	return windows.Handle(handle.Fd()), nil
}

func checkPipeServer(connection net.Conn) error {
	handle, err := getPipeHandle(connection)
	if err != nil {
		return err
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &pid); err != nil {
		return fmt.Errorf("localsocket: reading the pipe server: %w", err)
	}
	return checkSameUser(int(pid))
}

// GetPipeClientPID returns the process on the other end of an accepted pipe, refusing one that runs as another user.
func GetPipeClientPID(connection net.Conn) (int, error) {
	handle, err := getPipeHandle(connection)
	if err != nil {
		return 0, err
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(handle, &pid); err != nil {
		return 0, fmt.Errorf("localsocket: reading the pipe client: %w", err)
	}
	if err := checkSameUser(int(pid)); err != nil {
		return 0, err
	}
	return int(pid), nil
}
