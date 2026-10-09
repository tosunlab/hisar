package agent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
)

type Status struct {
	SocketPath        string `json:"socketPath"`
	IsRunning         bool   `json:"isRunning"`
	ErrorMessage      string `json:"errorMessage"`
	IsSSHConfigured   bool   `json:"isSSHConfigured"`
	ConfigPath        string `json:"configPath"`
	ConfigSection     string `json:"configSection"`
	IsTouchIDApproval bool   `json:"isTouchIDApproval"`
	BackupPath        string `json:"backupPath"`
}

type Config struct {
	HomeDirectory string
	SocketPath    string
	Server        *sshagent.Server
	Approver      *Approver
	Now           func() time.Time
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
	return status
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

// RespondSSHApproval answers the approval window: Authorize (then Touch ID when available) or Deny.
func (s *Service) RespondSSHApproval(requestID string, isAuthorized, isForAllApplication bool) {
	if s.config.Approver != nil {
		s.config.Approver.Respond(requestID, isAuthorized, isForAllApplication)
	}
}

func (s *Service) SetSSHApprovalForAllApplication(requestID string, isForAllApplication bool) {
	if s.config.Approver != nil {
		s.config.Approver.SetForAllApplication(requestID, isForAllApplication)
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
