package localsocket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
)

// maximumReadAhead holds one message of the largest size the SSH agent and command-line protocols allow (16 MiB),
// with its length.
const maximumReadAhead = 16<<20 + 4

var errReadAheadFull = errors.New("localsocket: the peer sent more than its requests")

const UnknownPeerName = "unknown"

// PeerInfo is the program on the other end of a connection; ApplicationPath is the app it runs in, when the resolver knows it.
type PeerInfo struct {
	PID             int
	Path            string
	Name            string
	ApplicationPath string
}

type PeerResolver interface {
	ResolvePeer(connection net.Conn) (PeerInfo, error)
}

// WatchConnectionClose reads the connection ahead of its user, so a peer that hangs up cancels the request it is waiting
// for, also with bytes it sent after the request still unread. It keeps at most maximumReadAhead unread bytes and
// cancels a peer that sends more; it stops once the connection is closed, which the caller does when cancel runs.
func WatchConnectionClose(connection net.Conn, cancel context.CancelFunc) io.Reader {
	reader := &readAhead{}
	reader.ready = sync.NewCond(&reader.mutex)
	go func() {
		chunk := make([]byte, 32<<10)
		for {
			count, err := connection.Read(chunk)
			reader.mutex.Lock()
			reader.buffer.Write(chunk[:count])
			if err == nil && reader.buffer.Len() > maximumReadAhead {
				err = errReadAheadFull
			}
			reader.err = err
			reader.ready.Broadcast()
			reader.mutex.Unlock()
			if err != nil {
				cancel()
				return
			}
		}
	}()
	return reader
}

// readAhead hands out what WatchConnectionClose read, then the error that ended the reading.
type readAhead struct {
	mutex  sync.Mutex
	ready  *sync.Cond
	buffer bytes.Buffer
	err    error
}

func (reader *readAhead) Read(target []byte) (int, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	for reader.buffer.Len() == 0 && reader.err == nil {
		reader.ready.Wait()
	}
	if reader.buffer.Len() > 0 {
		return reader.buffer.Read(target)
	}
	return 0, reader.err
}
