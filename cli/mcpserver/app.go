package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

var (
	ErrDeniedInHisar  = errors.New("The user denied access in Hisar.")
	ErrDialogTimedOut = errors.New("The user did not answer Hisar's approval dialog in time.")
)

// AppConnection is one connection to the Hisar app's socket; requests on it are sent one at a time.
type AppConnection interface {
	Call(ctx context.Context, request cliprotocol.Request, result any) error
	Close() error
}

// AppDialer connects to the running Hisar app.
type AppDialer func(ctx context.Context) (AppConnection, error)

type socketConnection struct {
	connection net.Conn
	timeout    time.Duration
	lastID     int
}

// CreateSocketConnection speaks the CLI protocol on a connection the caller opened; timeout bounds each request.
func CreateSocketConnection(connection net.Conn, timeout time.Duration) AppConnection {
	return &socketConnection{connection: connection, timeout: timeout}
}

func (connection *socketConnection) Call(ctx context.Context, request cliprotocol.Request, result any) error {
	connection.lastID++
	request.ID = strconv.Itoa(connection.lastID)
	connection.connection.SetDeadline(time.Now().Add(connection.timeout))
	stopClosing := context.AfterFunc(ctx, func() { connection.connection.SetDeadline(time.Now()) })
	defer stopClosing()
	if err := cliprotocol.WriteJSON(connection.connection, request); err != nil {
		return describeConnectionError(ctx, err)
	}
	var response cliprotocol.Response
	if err := cliprotocol.ReadJSON(connection.connection, &response); err != nil {
		return describeConnectionError(ctx, err)
	}
	if response.ID != request.ID {
		return errors.New("the Hisar app answered another request")
	}
	if response.Error != nil {
		return response.Error
	}
	if result != nil && len(response.Result) > 0 {
		return json.Unmarshal(response.Result, result)
	}
	return nil
}

func (connection *socketConnection) Close() error {
	return connection.connection.Close()
}

func describeConnectionError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return errors.New("the Hisar app did not answer in time")
	}
	return fmt.Errorf("lost the connection to the Hisar app: %w", err)
}

// appSession connects on the first call, introduces the agent, and connects again after a lost connection.
type appSession struct {
	dial       AppDialer
	mutex      sync.Mutex
	connection AppConnection
}

func (session *appSession) call(ctx context.Context, identity cliprotocol.Request, request cliprotocol.Request, result any) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.connection == nil {
		connection, err := session.dial(ctx)
		if err != nil {
			return fmt.Errorf("Hisar is not available: %v. Ask the user to open the Hisar app and to keep Settings → Command-line tool → “Let the command-line tool use Hisar” on.", err)
		}
		identity.Type = cliprotocol.TypeAgentHello
		if err := connection.Call(ctx, identity, nil); err != nil {
			connection.Close()
			return describeAppError(err)
		}
		session.connection = connection
	}
	err := session.connection.Call(ctx, request, result)
	var responseError *cliprotocol.ResponseError
	if err != nil && !errors.As(err, &responseError) {
		session.connection.Close()
		session.connection = nil
	}
	return describeAppError(err)
}

func (session *appSession) close() {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.connection != nil {
		session.connection.Close()
		session.connection = nil
	}
}

// describeAppError turns the app's answers into messages a model can relay to the user.
func describeAppError(err error) error {
	var responseError *cliprotocol.ResponseError
	if !errors.As(err, &responseError) {
		return err
	}
	switch responseError.Code {
	case cliprotocol.ErrorDenied:
		if strings.Contains(responseError.Message, "not answered") {
			return ErrDialogTimedOut
		}
		return ErrDeniedInHisar
	case cliprotocol.ErrorSignedOut:
		return errors.New("Hisar is not signed in. Ask the user to sign in to the Hisar app.")
	case cliprotocol.ErrorIntegrationDisabled:
		return errors.New("The Hisar app does not let the command-line tool use it. Ask the user to turn on Settings → Command-line tool → “Let the command-line tool use Hisar”.")
	case cliprotocol.ErrorLocked:
		return errors.New("Hisar is locked. Ask the user to unlock the Hisar app.")
	}
	return errors.New(responseError.Message)
}
