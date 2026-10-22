package agent

import (
	"errors"
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
// remembered per app, and the agent session it runs inside, read as the command-line socket reads it and joined in the
// live sessions both sockets share. A peer without a terminal in an app that hosted an agent session, whose chain has
// no harness or changed while it was read, may be work an agent handed off, so no remembered approval covers it.
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
	// AgentSessions are the live agent sessions the command-line socket shares (clisocket.NewSessionApprovals).
	AgentSessions *agentharness.Sessions
}

// ResolvePeer fails for a peer whose process chain cannot be read, as the command-line socket refuses one: a harness
// could run in it, so it must get neither the keys of every vault nor an app's remembered approval.
func (resolver PeerResolver) ResolvePeer(connection net.Conn) (sshagent.Peer, error) {
	info, err := resolver.Base.ResolvePeer(connection)
	if err != nil {
		return sshagent.Peer{}, err
	}
	if info.PID <= 0 {
		return sshagent.Peer{}, errors.New("agent: the peer's process is unknown")
	}
	peer := sshagent.Peer{PeerInfo: info}
	requester := resolver.Resolve(info.PID)
	peer.ApplicationPath = requester.App.Path
	session, isInside, err := agentharness.Find(resolver.ReadChain(info.PID))
	if err != nil {
		return sshagent.Peer{}, err
	}
	if isInside {
		host := ""
		if app, isVerified := resolver.IdentifyApp(requester); isVerified {
			resolver.AgentHosts.RecordAgentHost(app)
			host = app.Path
		}
		peer.AgentSession = resolver.AgentSessions.Join(session, host)
		return peer, nil
	}
	if !resolver.HasTerminal(info.PID) {
		if app, isVerified := resolver.IdentifyApp(requester); isVerified {
			peer.IsNeverRemembered = func() bool { return resolver.AgentHosts.IsAgentHost(app) }
		}
	}
	return peer, nil
}

// DescribeMissingAgentKey is the notification for an agent session that asked for SSH keys when no key is in a vault
// open to coding agents: its title and its text, which read as one message.
func DescribeMissingAgentKey(session agentharness.Session) (string, string) {
	return session.Harness.Name + " asked for an SSH key.", "No key is in a vault open to coding agents."
}
