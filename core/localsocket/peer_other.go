//go:build !darwin && !linux

package localsocket

import (
	"errors"
	"net"
)

type SystemPeerResolver struct{}

func (SystemPeerResolver) ResolvePeer(net.Conn) (PeerInfo, error) {
	return PeerInfo{Name: UnknownPeerName}, nil
}

func GetParentPID(pid int) (int, error) {
	return 0, errors.New("localsocket: parent processes are not available on this platform")
}

func GetProcessPath(pid int) (string, error) {
	return "", errors.New("localsocket: process paths are not available on this platform")
}
