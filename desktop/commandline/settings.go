package commandline

import (
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// Runner serves the CLI socket while the integration is on.
type Runner interface {
	Start()
	Stop()
	GetStatus() clisocket.RunnerStatus
}

// Status is Settings → Developer tools → Command-line tool: the installed hisar, the PATH, and the app integration.
type Status struct {
	IsAvailable              bool           `json:"isAvailable"`
	BundledPath              string         `json:"bundledPath"`
	InstalledPath            string         `json:"installedPath"`
	Installations            []Installation `json:"allInstallation"`
	PathCommandPath          string         `json:"pathCommandPath"`
	PathCommandTarget        string         `json:"pathCommandTarget"`
	IsPathCommandFromThisApp bool           `json:"isPathCommandFromThisApp"`
	PathHint                 string         `json:"pathHint"`
	IsIntegrationEnabled     bool           `json:"isIntegrationEnabled"`
	SocketPath               string         `json:"socketPath"`
	IsServing                bool           `json:"isServing"`
	ErrorMessage             string         `json:"errorMessage"`
}

// LauncherApprovals is the launcher memory of the CLI socket, clisocket.SessionApprovals.
type LauncherApprovals interface {
	ListLaunchers() []clisocket.LauncherApproval
	RevokeLauncher(appPath string)
}

// LauncherApp is an app approved as a launcher, as Settings → Developer tools → Command-line tool lists it; LastUsedAt
// is RFC 3339.
type LauncherApp struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IconURL    string `json:"iconURL"`
	LastUsedAt string `json:"lastUsedAt"`
}

// Service is bound to the UI for Settings → Developer tools → Command-line tool.
type Service struct {
	appService  *app.Service
	installer   *Installer
	runner      Runner
	launchers   LauncherApprovals
	describeApp func(bundlePath string) appinfo.App
}

// NewService takes the launcher memory, and appinfo's DescribeApp for the names and icons of the approved launchers.
func NewService(appService *app.Service, installer *Installer, runner Runner, launchers LauncherApprovals, describeApp func(bundlePath string) appinfo.App) *Service {
	return &Service{appService: appService, installer: installer, runner: runner, launchers: launchers, describeApp: describeApp}
}

// StartIntegration serves the socket at launch unless the switch is off; not a method, so the UI cannot call it.
func StartIntegration(service *Service) {
	if service.IsIntegrationEnabled() {
		service.runner.Start()
	}
}

func (service *Service) IsIntegrationEnabled() bool {
	return !app.ReadSettings(service.appService).IsCommandLineIntegrationDisabled
}

func (service *Service) buildStatus(installStatus InstallStatus) Status {
	runnerStatus := service.runner.GetStatus()
	return Status{
		IsAvailable:              installStatus.IsAvailable,
		BundledPath:              installStatus.BundledPath,
		InstalledPath:            installStatus.InstalledPath,
		Installations:            installStatus.Installations,
		PathCommandPath:          installStatus.PathCommandPath,
		PathCommandTarget:        installStatus.PathCommandTarget,
		IsPathCommandFromThisApp: installStatus.IsPathCommandFromThisApp,
		PathHint:                 installStatus.PathHint,
		IsIntegrationEnabled:     service.IsIntegrationEnabled(),
		SocketPath:               runnerStatus.SocketPath,
		IsServing:                runnerStatus.IsRunning,
		ErrorMessage:             runnerStatus.ErrorMessage,
	}
}

func (service *Service) GetCommandLineStatus() Status {
	return service.buildStatus(service.installer.GetStatus())
}

func (service *Service) InstallCommandLineTool() (Status, error) {
	installStatus, err := service.installer.Install()
	return service.buildStatus(installStatus), err
}

func (service *Service) UninstallCommandLineTool() (Status, error) {
	installStatus, err := service.installer.Uninstall()
	return service.buildStatus(installStatus), err
}

// SetCommandLineIntegration saves the switch, then opens or closes the socket; turning it off ends every approval.
func (service *Service) SetCommandLineIntegration(isEnabled bool) (Status, error) {
	err := app.UpdateSettings(service.appService, func(settings *localstate.Settings) { settings.IsCommandLineIntegrationDisabled = !isEnabled })
	if isEnabled {
		service.runner.Start()
	} else {
		service.runner.Stop()
	}
	return service.GetCommandLineStatus(), err
}

// ListLauncherApps lists the apps approved as launchers, most recently used first, once per app path: revoking is by
// path.
func (service *Service) ListLauncherApps() []LauncherApp {
	launchers := service.launchers.ListLaunchers()
	slices.SortStableFunc(launchers, func(first, second clisocket.LauncherApproval) int { return second.LastUsedAt.Compare(first.LastUsedAt) })
	launcherApps := []LauncherApp{}
	isListedByPath := map[string]bool{}
	for _, launcher := range launchers {
		if isListedByPath[launcher.App.Path] {
			continue
		}
		isListedByPath[launcher.App.Path] = true
		describedApp := service.describeApp(launcher.App.Path)
		launcherApps = append(launcherApps, LauncherApp{
			Name:       describedApp.Name,
			Path:       launcher.App.Path,
			IconURL:    describedApp.IconURL,
			LastUsedAt: launcher.LastUsedAt.UTC().Format(time.RFC3339),
		})
	}
	return launcherApps
}

// RevokeLauncherApp ends the launcher approval of the app at appPath, so it asks again, and returns the apps still
// approved.
func (service *Service) RevokeLauncherApp(appPath string) []LauncherApp {
	service.launchers.RevokeLauncher(appPath)
	return service.ListLauncherApps()
}
