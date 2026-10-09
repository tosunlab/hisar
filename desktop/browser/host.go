package browser

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"sync"
	"time"
)

func GetDefaultSocketPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".hisar", "browser.sock")
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
		return answerAllUnavailable(input, output)
	}
	defer connection.Close()
	connectFrame, err := json.Marshal(connectMessage{Type: "connect", ExtensionOrigin: config.ExtensionOrigin})
	if err != nil {
		return err
	}
	if err := WriteMessage(connection, connectFrame); err != nil {
		return answerAllUnavailable(input, output)
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		relayAllMessage(connection, output)
	}()
	err = relayAllMessage(input, connection)
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

// relayAllMessage copies whole frames, so an oversized or truncated message never reaches the other side.
func relayAllMessage(reader io.Reader, writer io.Writer) error {
	for {
		message, err := ReadMessage(reader)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := WriteMessage(writer, message); err != nil {
			return err
		}
	}
}

func answerAllUnavailable(input io.Reader, output io.Writer) error {
	for {
		message, err := ReadMessage(input)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var request Request
		json.Unmarshal(message, &request)
		response, err := json.Marshal(Response{ID: request.ID, Error: CreateError(ErrorDesktopUnavailable, "Hisar is not running")})
		if err != nil {
			return err
		}
		if err := WriteMessage(output, response); err != nil {
			return err
		}
	}
}
