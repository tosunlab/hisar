//go:build !darwin && !linux

package sshagent

import "net"

type SystemPeerResolver struct{}

func (SystemPeerResolver) ResolvePeer(net.Conn) (PeerInfo, error) {
	return PeerInfo{Name: UnknownPeerName}, nil
}
