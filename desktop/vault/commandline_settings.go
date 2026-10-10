package vault

import "github.com/tryhisar/hisar/desktop/commandline"

type CommandLineRunner interface {
	Start()
	Stop()
	GetStatus() commandline.RunnerStatus
}

// CommandLineStatus is Settings → Command-line tool: the installed hisar, the PATH, and the app integration.
type CommandLineStatus struct {
	IsAvailable              bool                       `json:"isAvailable"`
	BundledPath              string                     `json:"bundledPath"`
	InstalledPath            string                     `json:"installedPath"`
	AllInstallation          []commandline.Installation `json:"allInstallation"`
	PathCommandPath          string                     `json:"pathCommandPath"`
	PathCommandTarget        string                     `json:"pathCommandTarget"`
	IsPathCommandFromThisApp bool                       `json:"isPathCommandFromThisApp"`
	PathHint                 string                     `json:"pathHint"`
	IsIntegrationEnabled     bool                       `json:"isIntegrationEnabled"`
	SocketPath               string                     `json:"socketPath"`
	IsServing                bool                       `json:"isServing"`
	ErrorMessage             string                     `json:"errorMessage"`
}

// CommandLineService is bound to the UI for Settings → Command-line tool.
type CommandLineService struct {
	service   *Service
	installer *commandline.Installer
	runner    CommandLineRunner
}

func CreateCommandLineService(service *Service, installer *commandline.Installer, runner CommandLineRunner) *CommandLineService {
	return &CommandLineService{service: service, installer: installer, runner: runner}
}

// StartCommandLineIntegration serves the socket at launch unless the switch is off; not a method, so the UI cannot call it.
func StartCommandLineIntegration(commandLineService *CommandLineService) {
	if commandLineService.IsIntegrationEnabled() {
		commandLineService.runner.Start()
	}
}

func (commandLineService *CommandLineService) IsIntegrationEnabled() bool {
	s := commandLineService.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return !s.state.IsCommandLineIntegrationDisabled
}

func (commandLineService *CommandLineService) buildStatus(installStatus commandline.InstallStatus) CommandLineStatus {
	runnerStatus := commandLineService.runner.GetStatus()
	return CommandLineStatus{
		IsAvailable:              installStatus.IsAvailable,
		BundledPath:              installStatus.BundledPath,
		InstalledPath:            installStatus.InstalledPath,
		AllInstallation:          installStatus.AllInstallation,
		PathCommandPath:          installStatus.PathCommandPath,
		PathCommandTarget:        installStatus.PathCommandTarget,
		IsPathCommandFromThisApp: installStatus.IsPathCommandFromThisApp,
		PathHint:                 installStatus.PathHint,
		IsIntegrationEnabled:     commandLineService.IsIntegrationEnabled(),
		SocketPath:               runnerStatus.SocketPath,
		IsServing:                runnerStatus.IsRunning,
		ErrorMessage:             runnerStatus.ErrorMessage,
	}
}

func (commandLineService *CommandLineService) GetCommandLineStatus() CommandLineStatus {
	return commandLineService.buildStatus(commandLineService.installer.GetStatus())
}

func (commandLineService *CommandLineService) InstallCommandLineTool() (CommandLineStatus, error) {
	installStatus, err := commandLineService.installer.Install()
	return commandLineService.buildStatus(installStatus), err
}

func (commandLineService *CommandLineService) UninstallCommandLineTool() (CommandLineStatus, error) {
	installStatus, err := commandLineService.installer.Uninstall()
	return commandLineService.buildStatus(installStatus), err
}

// SetCommandLineIntegration saves the switch, then opens or closes the socket; turning it off ends every approval.
func (commandLineService *CommandLineService) SetCommandLineIntegration(isEnabled bool) (CommandLineStatus, error) {
	s := commandLineService.service
	s.mutex.Lock()
	s.state.IsCommandLineIntegrationDisabled = !isEnabled
	err := s.state.Save()
	s.mutex.Unlock()
	if isEnabled {
		commandLineService.runner.Start()
	} else {
		commandLineService.runner.Stop()
	}
	return commandLineService.GetCommandLineStatus(), err
}
