package app

import "errors"

const updateFeedPath = "/api/v1/desktop/updates/appcast.xml"

var (
	errUpdatesUnavailable = errors.New("updates are only available in the packaged Hisar app")
	errUpdateSignedOut    = errors.New("sign in to check for updates")
)

// Updater is the platform's self-updater (Sparkle on macOS).
type Updater interface {
	IsAvailable() bool
	GetVersion() (version, buildNumber string)
	// SetSession points the updater at the signed-in server; empty values mean signed out, and checks are skipped.
	SetSession(feedURL, authorization string)
	SetAutomaticCheck(isEnabled bool)
	CheckForUpdates() error
}

// UpdateNotifier is an updater that finds a newer version without installing it, on Linux and Windows: the user downloads it.
type UpdateNotifier interface {
	GetAvailableUpdate() AvailableUpdate
}

// AvailableUpdate is what an UpdateNotifier found; Version is empty while this version is the newest.
type AvailableUpdate struct {
	Version     string
	DownloadURL string
	HasChecked  bool
}

type UpdateStatus struct {
	Version                 string `json:"version"`
	BuildNumber             string `json:"buildNumber"`
	OperatingSystem         string `json:"operatingSystem"`
	IsAvailable             bool   `json:"isAvailable"`
	IsSignedIn              bool   `json:"isSignedIn"`
	IsAutomaticCheckEnabled bool   `json:"isAutomaticCheckEnabled"`
	// IsNotifyOnly: checking finds AvailableVersion and its DownloadURL, and the user installs it; otherwise the updater installs.
	IsNotifyOnly     bool   `json:"isNotifyOnly"`
	HasChecked       bool   `json:"hasChecked"`
	AvailableVersion string `json:"availableVersion"`
	DownloadURL      string `json:"downloadURL"`
}

// UpdateService is bound to the UI for Settings → General → Updates.
type UpdateService struct {
	service *Service
	updater Updater
}

// NewUpdateService hands the updater the current session now and on every sign-in and sign-out.
func NewUpdateService(service *Service, updater Updater) *UpdateService {
	updateService := &UpdateService{service: service, updater: updater}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	service.onSessionChange = updateService.applySession
	updateService.applySession(service.state.Server, service.state.Token, service.state.IsLoggedIn())
	updater.SetAutomaticCheck(!service.state.IsAutomaticUpdateCheckDisabled)
	return updateService
}

// applySession runs with the service mutex held, so the updater must not wait for the UI thread.
func (updateService *UpdateService) applySession(server, token string, isSignedIn bool) {
	if !isSignedIn {
		updateService.updater.SetSession("", "")
		return
	}
	updateService.updater.SetSession(server+updateFeedPath, "Bearer "+token)
}

func (updateService *UpdateService) GetUpdateStatus() UpdateStatus {
	version, buildNumber := updateService.updater.GetVersion()
	s := updateService.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	status := UpdateStatus{
		Version:                 version,
		BuildNumber:             buildNumber,
		OperatingSystem:         getOperatingSystem(),
		IsAvailable:             updateService.updater.IsAvailable(),
		IsSignedIn:              s.state.IsLoggedIn(),
		IsAutomaticCheckEnabled: !s.state.IsAutomaticUpdateCheckDisabled,
	}
	if notifier, isNotifier := updateService.updater.(UpdateNotifier); isNotifier {
		available := notifier.GetAvailableUpdate()
		status.IsNotifyOnly, status.HasChecked, status.AvailableVersion, status.DownloadURL = true, available.HasChecked, available.Version, available.DownloadURL
	}
	return status
}

// CheckForUpdates starts a check; Sparkle shows its own windows for the result, and a notifier's is in the status.
func (updateService *UpdateService) CheckForUpdates() (UpdateStatus, error) {
	status := updateService.GetUpdateStatus()
	if !status.IsAvailable {
		return status, errUpdatesUnavailable
	}
	if !status.IsSignedIn {
		return status, errUpdateSignedOut
	}
	err := updateService.updater.CheckForUpdates()
	return updateService.GetUpdateStatus(), err
}

func (updateService *UpdateService) SetAutomaticUpdateCheck(isEnabled bool) (UpdateStatus, error) {
	s := updateService.service
	s.mutex.Lock()
	s.state.IsAutomaticUpdateCheckDisabled = !isEnabled
	err := s.state.Save()
	s.mutex.Unlock()
	updateService.updater.SetAutomaticCheck(isEnabled)
	return updateService.GetUpdateStatus(), err
}
