package agent

import (
	"net"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// AgentHosts are the apps that hosted an agent session, kept by the command-line socket (clisocket.Server), so that
// recording one from either socket also ends its approvals as an app there.
type AgentHosts interface {
	IsAgentHost(app clisocket.AppIdentity) bool
	RecordAgentHost(app clisocket.AppIdentity)
}

// PeerResolver resolves the SSH agent's peers from their process chain: the app a peer runs in, so approvals are
// remembered per app, and the agent session it runs inside, read as the command-line socket reads it. A peer without a
// terminal in an app that hosted an agent session, whose chain has no harness or changed while it was read, may be work
// an agent handed off, so no remembered approval covers it.
type PeerResolver struct {
	Base localsocket.PeerResolver
	// Resolve finds the app responsible for a process (appinfo.Resolver.Resolve).
	Resolve func(pid int) appinfo.Requester
	// ReadChain reads a process chain as a snapshot of process instances (appinfo.Resolver.ReadChain).
	ReadChain   func(pid int) agentharness.Chain
	HasTerminal func(pid int) bool
	// IdentifyApp is the app as macOS verified its code signature (commandline.IdentifyApp).
	IdentifyApp func(requester appinfo.Requester) (clisocket.AppIdentity, bool)
	// AgentHosts are the apps that hosted an agent session; a peer inside a session records its app.
	AgentHosts AgentHosts
}

func (resolver PeerResolver) ResolvePeer(connection net.Conn) (sshagent.Peer, error) {
	info, err := resolver.Base.ResolvePeer(connection)
	peer := sshagent.Peer{PeerInfo: info}
	if err != nil || info.PID <= 0 {
		return peer, err
	}
	requester := resolver.Resolve(info.PID)
	peer.ApplicationPath = requester.App.Path
	if session, isInside, _ := agentharness.Find(resolver.ReadChain(info.PID)); isInside {
		peer.AgentSession = &session
		if app, isVerified := resolver.IdentifyApp(requester); isVerified {
			resolver.AgentHosts.RecordAgentHost(app)
		}
		return peer, nil
	}
	if !resolver.HasTerminal(info.PID) {
		app, isVerified := resolver.IdentifyApp(requester)
		peer.IsNeverRemembered = isVerified && resolver.AgentHosts.IsAgentHost(app)
	}
	return peer, nil
}

// DescribeMissingAgentKey is the notification for an agent session that asked for SSH keys when no key is in a vault
// open to coding agents: its title and its text, which read as one message.
func DescribeMissingAgentKey(session agentharness.Session) (string, string) {
	return session.Harness.Name + " asked for an SSH key.", "No key is in a vault open to coding agents."
}
