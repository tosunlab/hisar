package apiclient

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

// Event is one server-sent event from GET /api/v1/events.
type Event struct {
	Name string
	Data []byte
}

// EventIdleTimeout ends a stream that sent nothing, not even a heartbeat, for this long.
const EventIdleTimeout = 90 * time.Second

var ErrEventStreamIdle = errors.New("client: the event stream went quiet")

// StreamEvents calls handleOpen, when not nil, once the server has accepted the stream, then handleEvent for each event
// until ctx ends, the server closes the stream or it goes quiet. The server keeps no backlog: a change made before
// handleOpen is on no stream.
func (c *Client) StreamEvents(ctx context.Context, handleOpen func(), handleEvent func(Event)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	if c.Token != "" {
		request.Header.Set("Authorization", "Bearer "+c.Token)
	}
	c.setClientHeader(request.Header)
	// The regular client's overall timeout would cut a long-lived stream; idleness is checked instead.
	streamClient := &http.Client{Transport: c.HTTP.Transport, CheckRedirect: CheckRedirect}
	response, err := streamClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if response.StatusCode == http.StatusUpgradeRequired {
			return c.readUpgradeRequired(data)
		}
		if response.StatusCode == http.StatusForbidden {
			return readError(response.StatusCode, data)
		}
		return &APIError{Status: response.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	if handleOpen != nil {
		handleOpen()
	}

	idleTimer := time.AfterFunc(EventIdleTimeout, cancel)
	defer idleTimer.Stop()
	isIdle := func() bool { return !idleTimer.Stop() }
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), apitypes.MaxEventSize)
	var eventName string
	var dataLines []string
	for scanner.Scan() {
		if isIdle() {
			return ErrEventStreamIdle
		}
		idleTimer.Reset(EventIdleTimeout)
		line := scanner.Text()
		switch {
		case line == "":
			if eventName != "" || len(dataLines) > 0 {
				handleEvent(Event{Name: eventName, Data: []byte(strings.Join(dataLines, "\n"))})
			}
			eventName, dataLines = "", nil
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if isIdle() {
		return ErrEventStreamIdle
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return ctx.Err()
}
