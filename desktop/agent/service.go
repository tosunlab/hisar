package agent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/localsocket"
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
	SSHRememberSeconds     int    `json:"sshRememberSeconds"`
	BackupPath             string `json:"backupPath"`
}

// ApprovalPreference stores the "Compact SSH approvals", "Approve requests on my smart device", "Remember commit signing"
// and "Remember SSH approvals" settings; SSH remember seconds below 0 mean until Hisar locks.
type ApprovalPreference interface {
	IsCompactSSHApproval() bool
	SetCompactSSHApproval(isCompact bool) error
	IsPhoneApproval() bool
	SetPhoneApproval(isEnabled bool) error
	GetSigningRememberSeconds() int
	SetSigningRememberSeconds(seconds int) error
	GetSSHRememberSeconds() int
	SetSSHRememberSeconds(seconds int) error
}

// GetSSHRememberDuration converts the "Remember SSH approvals" setting to the agent's approval lifetime: 0 keeps approvals until Hisar locks.
func GetSSHRememberDuration(seconds int) time.Duration {
	if seconds < 0 {
		return 0
	}
	if seconds == 0 {
		return sshagent.DefaultApprovalLifetime
	}
	return time.Duration(seconds) * time.Second
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

func NewService(config Config) *Service {
	if config.SocketPath == "" {
		config.SocketPath = GetDefaultSocketPath(config.HomeDirectory)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	service := &Service{config: config}
	service.applySSHRemember()
	return service
}

func (s *Service) applySSHRemember() {
	if s.config.Server != nil && s.config.ApprovalPreference != nil {
		s.config.Server.SetApprovalLifetime(GetSSHRememberDuration(s.config.ApprovalPreference.GetSSHRememberSeconds()))
	}
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
		status.SSHRememberSeconds = s.config.ApprovalPreference.GetSSHRememberSeconds()
	}
	return status
}

// SetSSHRemember stores "Remember SSH approvals" (3600 seconds, or -1 for until Hisar locks) and applies it to the agent's remembered approvals.
func (s *Service) SetSSHRemember(seconds int) (Status, error) {
	if s.config.ApprovalPreference == nil {
		return s.buildStatus(), errors.New("remembering SSH approvals cannot be changed here")
	}
	err := s.config.ApprovalPreference.SetSSHRememberSeconds(seconds)
	s.applySSHRemember()
	return s.buildStatus(), err
}

// SetSigningRemember stores "Remember commit signing" (0, 600 or 3600 seconds) and forgets every remembered signature.
func (s *Service) SetSigningRemember(seconds int) (Status, error) {
	if s.config.ApprovalPreference == nil {
		return s.buildStatus(), errors.New("commit signing cannot be changed here")
	}
	err := s.config.ApprovalPreference.SetSigningRememberSeconds(seconds)
	if s.config.Approver != nil {
		s.config.Approver.ClearRememberedSignings()
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

// SetPhoneApproval turns "Approve requests on my smart device" on or off for dialogs that open from now on.
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

// SetSSHApprovalGrantDuration follows an agent dialog's duration menu; a change restarts a Touch ID check in progress.
func (s *Service) SetSSHApprovalGrantDuration(requestID, duration string) {
	if s.config.Approver != nil {
		s.config.Approver.SetAgentGrantDuration(requestID, duration)
	}
}

// SetSSHApprovalLogin follows a sign-in dialog's choice of login; a change restarts a Touch ID check in progress.
func (s *Service) SetSSHApprovalLogin(requestID string, loginIndex int) {
	if s.config.Approver != nil {
		s.config.Approver.SetSignInLogin(requestID, loginIndex)
	}
}

func (s *Service) ListPendingSSHApprovals() []ApprovalEvent {
	if s.config.Approver == nil {
		return []ApprovalEvent{}
	}
	return s.config.Approver.ListPendingApprovals()
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
	listener, err := localsocket.ListenUnixSocket(service.config.SocketPath)
	if err != nil {
		service.setServing(false, err)
		return err
	}
	service.setServing(true, nil)
	err = service.config.Server.Serve(ctx, listener)
	service.setServing(false, err)
	return err
}
