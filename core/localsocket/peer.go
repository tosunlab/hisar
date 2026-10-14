package localsocket

import (
	"context"
	"io"
	"net"
)

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

// WatchConnectionClose reads the connection ahead of its user, so a peer that hangs up cancels the request it is waiting for.
func WatchConnectionClose(connection net.Conn, cancel context.CancelFunc) io.Reader {
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		_, err := io.Copy(pipeWriter, connection)
		cancel()
		pipeWriter.CloseWithError(err)
	}()
	return pipeReader
}
