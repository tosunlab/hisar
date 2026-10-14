package cliprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Client sends requests to the Hisar app on one connection, one at a time, numbering them.
type Client struct {
	connection net.Conn
	lastID     int
}

func NewClient(connection net.Conn) *Client {
	return &Client{connection: connection}
}

// Call sends request and decodes the answer into result; timeout bounds the round trip, and ctx can end it sooner.
// An error answer is returned as *ResponseError.
func (client *Client) Call(ctx context.Context, request Request, result any, timeout time.Duration) error {
	client.lastID++
	request.ID = strconv.Itoa(client.lastID)
	client.connection.SetDeadline(time.Now().Add(timeout))
	stopClosing := context.AfterFunc(ctx, func() { client.connection.SetDeadline(time.Now()) })
	defer stopClosing()
	if err := WriteJSON(client.connection, request); err != nil {
		return describeConnectionError(ctx, err)
	}
	var response Response
	if err := ReadJSON(client.connection, &response); err != nil {
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

func (client *Client) Close() error {
	return client.connection.Close()
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
