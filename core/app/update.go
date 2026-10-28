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

// UpdateNotifier is an updater that finds a newer version for the user to download, on Linux and Windows.
type UpdateNotifier interface {
	GetAvailableUpdate() AvailableUpdate
}

// UpdateInstaller is a notifier that also installs what it found: Linux Hisar from a package. GetInstallKind names
// the package ("appimage", "deb", "rpm", "flatpak"), empty for a copy that leaves the download to the user.
type UpdateInstaller interface {
	GetInstallKind() string
	InstallUpdate() error
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
	// InstallKind is how InstallUpdate installs AvailableVersion: "appimage" replaces Hisar and starts it again;
	// "deb", "rpm" and "flatpak" open the verified package in the system's software installer. Empty: the user
	// downloads it from DownloadURL.
	InstallKind string `json:"installKind"`
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
	if installer, isInstaller := updateService.updater.(UpdateInstaller); isInstaller {
		status.InstallKind = installer.GetInstallKind()
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

// InstallUpdate downloads and verifies the version a check found and installs it as InstallKind says; after an
// AppImage update Hisar quits and the new version starts.
func (updateService *UpdateService) InstallUpdate() (UpdateStatus, error) {
	installer, isInstaller := updateService.updater.(UpdateInstaller)
	if !isInstaller || installer.GetInstallKind() == "" {
		return updateService.GetUpdateStatus(), errors.New("this copy of Hisar cannot install updates; download the new version from its release")
	}
	err := installer.InstallUpdate()
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
