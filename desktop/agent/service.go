package agent

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
)

type Status struct {
	SocketPath             string `json:"socketPath"`
	IsRunning              bool   `json:"isRunning"`
	ErrorMessage           string `json:"errorMessage"`
	IsSSHConfigured        bool   `json:"isSSHConfigured"`
	ConfigPath             string `json:"configPath"`
	ConfigSection          string `json:"configSection"`
	IsTouchIDApproval      bool   `json:"isTouchIDApproval"`
	IsCompactApproval      bool   `json:"isCompactApproval"`
	IsPhoneApproval        bool   `json:"isPhoneApproval"`
	SigningRememberSeconds int    `json:"signingRememberSeconds"`
	BackupPath             string `json:"backupPath"`
}

// ApprovalPreference stores the "Compact SSH approvals", "Allow approvals from my iPhone" and "Remember commit signing" settings.
type ApprovalPreference interface {
	IsCompactSSHApproval() bool
	SetCompactSSHApproval(isCompact bool) error
	IsPhoneApproval() bool
	SetPhoneApproval(isEnabled bool) error
	GetSigningRememberSeconds() int
	SetSigningRememberSeconds(seconds int) error
}

type Config struct {
	HomeDirectory      string
	SocketPath         string
	Server             *sshagent.Server
	Approver           *Approver
	ApprovalPreference ApprovalPreference
	// FitApprovalWindow sizes the approval window to the page's content and places it, centered or at the top right.
	FitApprovalWindow func(isCompact bool, height int)
	Now               func() time.Time
}

// ApplicationPeerResolver adds the app a peer runs in, so approvals are remembered per app.
type ApplicationPeerResolver struct {
	Base                sshagent.PeerResolver
	FindApplicationPath func(pid int) (string, bool)
}

func (resolver ApplicationPeerResolver) ResolvePeer(connection net.Conn) (sshagent.PeerInfo, error) {
	peer, err := resolver.Base.ResolvePeer(connection)
	if err != nil || peer.PID <= 0 || resolver.FindApplicationPath == nil {
		return peer, err
	}
	if applicationPath, isFound := resolver.FindApplicationPath(peer.PID); isFound {
		peer.ApplicationPath = applicationPath
	}
	return peer, nil
}

// Service is bound to the UI; Serve runs the agent itself.
type Service struct {
	config     Config
	mutex      sync.Mutex
	isRunning  bool
	serveError error
}

func GetDefaultSocketPath(homeDirectory string) string {
	return filepath.Join(homeDirectory, ".hisar", "agent.sock")
}

func CreateService(config Config) *Service {
	if config.SocketPath == "" {
		config.SocketPath = GetDefaultSocketPath(config.HomeDirectory)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{config: config}
}

func (s *Service) buildStatus() Status {
	status := Status{
		SocketPath:    s.config.SocketPath,
		ConfigPath:    getSSHConfigPath(s.config.HomeDirectory),
		ConfigSection: BuildSSHConfigSection(s.config.HomeDirectory, s.config.SocketPath),
	}
	s.mutex.Lock()
	status.IsRunning = s.isRunning
	if s.serveError != nil {
		status.ErrorMessage = s.serveError.Error()
	}
	s.mutex.Unlock()
	isConfigured, err := IsSSHConfigured(s.config.HomeDirectory)
	status.IsSSHConfigured = isConfigured
	if err != nil && status.ErrorMessage == "" {
		status.ErrorMessage = "could not read " + status.ConfigPath + ": " + err.Error()
	}
	status.IsTouchIDApproval = s.config.Approver != nil && s.config.Approver.IsTouchIDApproval()
	status.IsCompactApproval = s.config.ApprovalPreference != nil && s.config.ApprovalPreference.IsCompactSSHApproval()
	status.IsPhoneApproval = s.config.ApprovalPreference != nil && s.config.ApprovalPreference.IsPhoneApproval()
	if s.config.ApprovalPreference != nil {
		status.SigningRememberSeconds = s.config.ApprovalPreference.GetSigningRememberSeconds()
	}
	return status
}

// SetSigningRemember stores "Remember commit signing" (0, 600 or 3600 seconds) and forgets every remembered signature.
func (s *Service) SetSigningRemember(seconds int) (Status, error) {
	if s.config.ApprovalPreference == nil {
		return s.buildStatus(), errors.New("commit signing cannot be changed here")
	}
	err := s.config.ApprovalPreference.SetSigningRememberSeconds(seconds)
	if s.config.Approver != nil {
		s.config.Approver.ClearAllRememberedSigning()
	}
	return s.buildStatus(), err
}

func (s *Service) SetCompactSSHApproval(isCompact bool) (Status, error) {
	if s.config.ApprovalPreference == nil {
		return s.buildStatus(), errors.New("the approval layout cannot be changed here")
	}
	err := s.config.ApprovalPreference.SetCompactSSHApproval(isCompact)
	return s.buildStatus(), err
}

// SetPhoneApproval turns "Allow approvals from my iPhone" on or off for dialogs that open from now on.
func (s *Service) SetPhoneApproval(isEnabled bool) (Status, error) {
	if s.config.ApprovalPreference == nil {
		return s.buildStatus(), errors.New("phone approvals cannot be changed here")
	}
	err := s.config.ApprovalPreference.SetPhoneApproval(isEnabled)
	return s.buildStatus(), err
}

// FitSSHApprovalWindow lets the page size the window to what it shows; the height is limited to the screen by the caller.
func (s *Service) FitSSHApprovalWindow(isCompact bool, height int) {
	if s.config.FitApprovalWindow != nil && height > 0 {
		s.config.FitApprovalWindow(isCompact, height)
	}
}

func (s *Service) GetStatus() Status {
	return s.buildStatus()
}

// ConfigureSSHConfig points ssh at the Hisar agent; the UI calls it only when the user asks.
func (s *Service) ConfigureSSHConfig() (Status, error) {
	backupPath, err := ConfigureSSHConfig(s.config.HomeDirectory, s.config.SocketPath, s.config.Now())
	status := s.buildStatus()
	status.BackupPath = backupPath
	return status, err
}

func (s *Service) RemoveSSHConfig() (Status, error) {
	backupPath, err := RemoveSSHConfig(s.config.HomeDirectory, s.config.Now())
	status := s.buildStatus()
	status.BackupPath = backupPath
	return status, err
}

// RespondSSHApproval answers the approval window: approve (then Touch ID when available) or Deny.
func (s *Service) RespondSSHApproval(requestID string, isAuthorized, isRemembered bool) {
	if s.config.Approver != nil {
		s.config.Approver.Respond(requestID, isAuthorized, isRemembered)
	}
}

// RespondApprovalWithPassword approves with the master password, which also unlocks a locked Hisar.
func (s *Service) RespondApprovalWithPassword(requestID, password string, isRemembered bool) {
	if s.config.Approver != nil {
		s.config.Approver.RespondWithPassword(requestID, password, isRemembered)
	}
}

func (s *Service) SetSSHApprovalRemembered(requestID string, isRemembered bool) {
	if s.config.Approver != nil {
		s.config.Approver.SetRemembered(requestID, isRemembered)
	}
}

func (s *Service) ListAllPendingSSHApproval() []ApprovalEvent {
	if s.config.Approver == nil {
		return []ApprovalEvent{}
	}
	return s.config.Approver.ListAllPendingApproval()
}

func (s *Service) setServing(isRunning bool, serveError error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isRunning = isRunning
	s.serveError = serveError
}

// Serve listens on the socket and blocks until ctx ends; a failure is also reported by GetStatus.
func Serve(ctx context.Context, service *Service) error {
	if service.config.Server == nil {
		err := errors.New("the SSH agent is not set up")
		service.setServing(false, err)
		return err
	}
	listener, err := sshagent.ListenUnixSocket(service.config.SocketPath)
	if err != nil {
		service.setServing(false, err)
		return err
	}
	service.setServing(true, nil)
	err = service.config.Server.Serve(ctx, listener)
	service.setServing(false, err)
	return err
}
