package vault

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
)

const (
	clientVersionCheckLifetime            = time.Hour
	unreachableClientVersionCheckLifetime = time.Minute
	versionCheckTimeout                   = 10 * time.Second
	clientVersionCheckTimeout             = 3 * time.Second
)

// UpdateRequirement is the server's minimum version for an app that is older; nil while the version is accepted.
type UpdateRequirement struct {
	CurrentVersion string `json:"currentVersion"`
	MinimumVersion string `json:"minimumVersion"`
	UpdateURL      string `json:"updateURL"`
}

type UpdateRequiredError struct {
	Requirement UpdateRequirement
}

func (e *UpdateRequiredError) Error() string {
	return fmt.Sprintf("Hisar needs an update: the server requires version %s or later. Open Hisar to update.", e.Requirement.MinimumVersion)
}

type clientVersionCheck struct {
	requirement *UpdateRequirement
	expiresAt   time.Time
}

type updateRequirementState struct {
	mutex                sync.Mutex
	requirement          *UpdateRequirement
	allClientCheckByName map[string]clientVersionCheck
}

// SetAppVersion is for apps that learn their version after creating the service; not a method, so the UI cannot call it.
func SetAppVersion(s *Service, version string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.config.AppVersion = version
}

func (s *Service) createClient(server string) *apiclient.Client {
	apiClient := apiclient.Create(server)
	apiClient.Platform, apiClient.Version = s.config.Platform, s.config.AppVersion
	apiClient.OnUpgradeRequired = s.recordUpdateRequired
	return apiClient
}

func (s *Service) recordUpdateRequired(upgradeRequired *apiclient.UpgradeRequiredError) {
	requirement := UpdateRequirement{CurrentVersion: s.config.AppVersion, MinimumVersion: upgradeRequired.MinimumVersion, UpdateURL: upgradeRequired.UpdateURL}
	s.updateRequirement.mutex.Lock()
	isChanged := s.updateRequirement.requirement == nil || *s.updateRequirement.requirement != requirement
	s.updateRequirement.requirement = &requirement
	s.updateRequirement.mutex.Unlock()
	if isChanged && s.config.OnUpdateRequired != nil {
		go s.config.OnUpdateRequired(requirement)
	}
}

func (s *Service) getUpdateRequirement() *UpdateRequirement {
	s.updateRequirement.mutex.Lock()
	defer s.updateRequirement.mutex.Unlock()
	if s.updateRequirement.requirement == nil {
		return nil
	}
	requirement := *s.updateRequirement.requirement
	return &requirement
}

func (s *Service) clearUpdateRequirement() {
	s.updateRequirement.mutex.Lock()
	defer s.updateRequirement.mutex.Unlock()
	s.updateRequirement.requirement = nil
}

// requireCurrentVersion refuses the SSH agent, the command-line tool, coding agents and the browser while Hisar needs an update.
func (s *Service) requireCurrentVersion() error {
	if requirement := s.getUpdateRequirement(); requirement != nil {
		return &UpdateRequiredError{Requirement: *requirement}
	}
	return nil
}

// CheckMinimumVersion asks the server whether it still accepts this version. Offline, Hisar keeps what it knew,
// and any answer other than 426 (including a server too old to know the check) clears an earlier requirement.
func (s *Service) CheckMinimumVersion() Status {
	s.mutex.Lock()
	server := s.state.Server
	s.mutex.Unlock()
	if server == "" {
		server = defaultServer
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionCheckTimeout)
	defer cancel()
	err := s.createClient(server).CheckVersion(ctx)
	if _, isUpgradeRequired := apiclient.AsUpgradeRequired(err); !isUpgradeRequired && !apiclient.IsUnreachable(err) {
		s.clearUpdateRequirement()
	}
	return s.GetStatus()
}

// checkClientVersion asks the server about another app that reaches it only through Hisar: the browser extension, or
// the command-line tool in app mode. Unreachable means accepted, as offline apps keep working.
func (s *Service) checkClientVersion(platform, version string) *UpdateRequirement {
	if version == "" {
		return nil
	}
	checkName := platform + " " + version
	now := s.config.Now()
	s.updateRequirement.mutex.Lock()
	if check, isCached := s.updateRequirement.allClientCheckByName[checkName]; isCached && now.Before(check.expiresAt) {
		s.updateRequirement.mutex.Unlock()
		return check.requirement
	}
	s.updateRequirement.mutex.Unlock()

	s.mutex.Lock()
	server := s.state.Server
	s.mutex.Unlock()
	if server == "" {
		return nil
	}
	apiClient := apiclient.Create(server)
	apiClient.Platform, apiClient.Version = platform, version
	ctx, cancel := context.WithTimeout(context.Background(), clientVersionCheckTimeout)
	defer cancel()
	err := apiClient.CheckVersion(ctx)
	check := clientVersionCheck{expiresAt: now.Add(clientVersionCheckLifetime)}
	if upgradeRequired, isUpgradeRequired := apiclient.AsUpgradeRequired(err); isUpgradeRequired {
		check.requirement = &UpdateRequirement{CurrentVersion: version, MinimumVersion: upgradeRequired.MinimumVersion, UpdateURL: upgradeRequired.UpdateURL}
	} else if apiclient.IsUnreachable(err) {
		check.expiresAt = now.Add(unreachableClientVersionCheckLifetime)
	}
	s.updateRequirement.mutex.Lock()
	if s.updateRequirement.allClientCheckByName == nil {
		s.updateRequirement.allClientCheckByName = map[string]clientVersionCheck{}
	}
	s.updateRequirement.allClientCheckByName[checkName] = check
	s.updateRequirement.mutex.Unlock()
	return check.requirement
}
