package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	progressInterval      = 500 * time.Millisecond
	maximumProgressLength = 16 << 10
)

// progressReporter sends notifications/progress for a tool call whose client sent a progressToken.
type progressReporter struct {
	session *mcp.ServerSession
	token   any
	mutex   sync.Mutex
	count   float64
}

// newProgressReporter returns nil when the client did not ask for progress.
func newProgressReporter(request *mcp.CallToolRequest) *progressReporter {
	token := request.Params.GetProgressToken()
	if token == nil || request.Session == nil {
		return nil
	}
	return &progressReporter{session: request.Session, token: token}
}

func (reporter *progressReporter) report(ctx context.Context, message string) {
	if reporter == nil {
		return
	}
	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()
	reporter.count++
	reporter.session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: reporter.token, Progress: reporter.count, Message: message})
}

// progressStream gathers a command's masked stdout and stderr and reports what arrived every progressInterval.
type progressStream struct {
	reporter      *progressReporter
	mutex         sync.Mutex
	pending       []byte
	omittedLength int
}

func (stream *progressStream) Write(chunk []byte) (int, error) {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	stream.pending = append(stream.pending, chunk...)
	if excess := len(stream.pending) - maximumProgressLength; excess > 0 {
		stream.omittedLength += excess
		stream.pending = append(stream.pending[:0], stream.pending[excess:]...)
	}
	return len(chunk), nil
}

// take returns what arrived since the last call; before the end it keeps back a character whose bytes have not
// all arrived.
func (stream *progressStream) take(isFinal bool) string {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	end := len(stream.pending)
	if !isFinal {
		end = findCompleteLength(stream.pending)
	}
	text := string(stream.pending[:end])
	if stream.omittedLength > 0 {
		text = fmt.Sprintf("[… %d bytes not shown …]\n", stream.omittedLength) + text
		stream.omittedLength = 0
	}
	stream.pending = append(stream.pending[:0], stream.pending[end:]...)
	return strings.ToValidUTF8(text, "�")
}

func (stream *progressStream) send(ctx context.Context, isFinal bool) {
	if text := stream.take(isFinal); text != "" {
		stream.reporter.report(ctx, text)
	}
}

// start reports the stream until the returned function is called, which reports the rest.
func (stream *progressStream) start(ctx context.Context) func() {
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				stream.send(ctx, false)
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-stopped
		stream.send(ctx, true)
	}
}

// findCompleteLength returns the length of data without a last character that is missing bytes.
func findCompleteLength(data []byte) int {
	for start := len(data) - 1; start >= max(0, len(data)-utf8.UTFMax); start-- {
		if utf8.RuneStart(data[start]) {
			if utf8.FullRune(data[start:]) {
				return len(data)
			}
			return start
		}
	}
	return len(data)
}
