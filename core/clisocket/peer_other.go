//go:build !darwin && !linux

package clisocket

import "errors"

var errUnsupportedPlatform = errors.New("commandline: peer credentials are not available on this platform")

func getPeerCredential(int) (Peer, error) {
	return Peer{}, errUnsupportedPlatform
}

func readProcessSession(int) (TerminalSession, error) {
	return TerminalSession{}, errUnsupportedPlatform
}

func findTerminalName(int64) string { return "" }
