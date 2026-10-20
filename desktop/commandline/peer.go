package commandline

import (
	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// PeerResolver is the CLI socket's peer resolver. It finds agent sessions in appinfo's chain, which also matches a
// harness by its code signature, so omp in Tern is found although its executable is named after its version.
type PeerResolver struct {
	clisocket.SystemPeerResolver
	Resolver *appinfo.Resolver
}

func (resolver PeerResolver) GetAgentSession(pid int) (agentharness.Session, bool, error) {
	return agentharness.Find(resolver.Resolver.ReadChain(pid))
}
