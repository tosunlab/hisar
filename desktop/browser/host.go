package browser

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

func GetDefaultSocketPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".hisar", "browser.sock")
}

// ReadCaller returns the extension that started the host, from its arguments. Chromium browsers pass the extension's
// origin first (and on Windows a parent window after it); Firefox passes the path of the host manifest, then the add-on
// ID.
func ReadCaller(arguments []string) string {
	switch {
	case len(arguments) > 1 && strings.HasPrefix(arguments[1], "chrome-extension://"):
		return arguments[1]
	case len(arguments) > 2:
		return arguments[2]
	case len(arguments) > 1:
		return arguments[1]
	}
	return ""
}

type HostConfig struct {
	SocketPath      string
	ExtensionOrigin string
	// LaunchApp starts the desktop app when its socket is missing; nil skips launching.
	LaunchApp  func() error
	RetryFor   time.Duration
	RetryEvery time.Duration
}

// RunHost relays native messaging frames between the browser (input, output) and the desktop app's socket.
func RunHost(input io.Reader, output io.Writer, config HostConfig) error {
	connection, err := dialDesktop(config)
	if err != nil {
		return answerUnavailable(input, output)
	}
	defer connection.Close()
	connectFrame, err := json.Marshal(browserprotocol.ConnectMessage{Type: "connect", ExtensionOrigin: config.ExtensionOrigin})
	if err != nil {
		return err
	}
	if err := browserprotocol.WriteMessage(connection, connectFrame); err != nil {
		return answerUnavailable(input, output)
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		relayMessages(connection, output)
	}()
	err = relayMessages(input, connection)
	if unixConnection, ok := connection.(*net.UnixConn); ok {
		unixConnection.CloseWrite()
	}
	waitGroup.Wait()
	return err
}

func dialDesktop(config HostConfig) (net.Conn, error) {
	connection, err := net.Dial("unix", config.SocketPath)
	if err == nil || config.LaunchApp == nil {
		return connection, err
	}
	if launchErr := config.LaunchApp(); launchErr != nil {
		return nil, errors.Join(err, launchErr)
	}
	retryEvery := config.RetryEvery
	if retryEvery == 0 {
		retryEvery = 250 * time.Millisecond
	}
	deadline := time.Now().Add(config.RetryFor)
	for time.Now().Before(deadline) {
		time.Sleep(retryEvery)
		if connection, err = net.Dial("unix", config.SocketPath); err == nil {
			return connection, nil
		}
	}
	return nil, err
}

// relayMessages copies whole frames, so an oversized or truncated message never reaches the other side.
func relayMessages(reader io.Reader, writer io.Writer) error {
	for {
		message, err := browserprotocol.ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := browserprotocol.WriteMessage(writer, message); err != nil {
			return err
		}
	}
}

func answerUnavailable(input io.Reader, output io.Writer) error {
	for {
		message, err := browserprotocol.ReadMessage(input)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var request browserprotocol.Request
		json.Unmarshal(message, &request)
		response, err := json.Marshal(browserprotocol.Response{ID: request.ID, Error: browserprotocol.NewError(browserprotocol.ErrorDesktopUnavailable, "Hisar is not running")})
		if err != nil {
			return err
		}
		if err := browserprotocol.WriteMessage(output, response); err != nil {
			return err
		}
	}
}
