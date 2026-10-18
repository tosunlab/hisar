package commandline

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
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
	IsInstalledByPackage     bool           `json:"isInstalledByPackage"`
	IsIntegrationEnabled     bool           `json:"isIntegrationEnabled"`
	SocketPath               string         `json:"socketPath"`
	IsServing                bool           `json:"isServing"`
	ErrorMessage             string         `json:"errorMessage"`
}

// Service is bound to the UI for Settings → Developer tools → Command-line tool.
type Service struct {
	appService *app.Service
	installer  *Installer
	runner     Runner
}

func NewService(appService *app.Service, installer *Installer, runner Runner) *Service {
	return &Service{appService: appService, installer: installer, runner: runner}
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
		IsInstalledByPackage:     installStatus.IsInstalledByPackage,
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
