package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/axfill"
	"github.com/tryhisar/hisar/desktop/background"
	"github.com/tryhisar/hisar/desktop/biometric"
	"github.com/tryhisar/hisar/desktop/browser"
	"github.com/tryhisar/hisar/desktop/commandline"
	"github.com/tryhisar/hisar/desktop/granthead"
	"github.com/tryhisar/hisar/desktop/harness"
	"github.com/tryhisar/hisar/desktop/kitreader"
	"github.com/tryhisar/hisar/desktop/loginitem"
	"github.com/tryhisar/hisar/desktop/pasteboard"
	"github.com/tryhisar/hisar/desktop/portable"
	"github.com/tryhisar/hisar/desktop/systemidle"
	"github.com/tryhisar/hisar/desktop/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	statePath, err := localstate.GetPath("HISAR_DESKTOP_STATE", "desktop.json")
	if err != nil {
		log.Fatal(err)
	}
	cachePath, err := localstate.GetPath("HISAR_DESKTOP_CACHE", "desktop-cache.db")
	if err != nil {
		log.Fatal(err)
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	fallbackClipboard := &wailsClipboard{}
	touchID := biometric.New("Hisar Touch ID")
	notifier := &appNotifier{}
	fileSaver := &wailsFileSaver{}
	fileOpener := &wailsFileOpener{}
	var chooseEmergencyKit func() (string, error)
	emergencyKitService := kitreader.New(func() (string, error) { return chooseEmergencyKit() })
	requesterResolver := appinfo.NewSystemResolver()
	var agentServer *sshagent.Server
	var approver *agent.Approver
	var showUpdateRequired func(requirement app.UpdateRequirement)
	var showItem func(location app.ItemLocation)
	vaultConfig := app.Config{
		StatePath:             statePath,
		CachePath:             cachePath,
		DeviceName:            getDeviceName(),
		Platform:              runtime.GOOS + "/desktop",
		AppVersion:            getAppVersion(),
		Clipboard:             pasteboard.New(fallbackClipboard),
		LockNotifier:          notifier,
		GetSystemIdleDuration: systemidle.GetIdleDuration,
		Biometric:             touchID,
		BiometricKeyPreparer:  touchID,
		BiometricName:         "Touch ID",
		FileSaver:             fileSaver,
		FileOpener:            fileOpener,
		OnLock: func() {
			if agentServer != nil {
				agentServer.ClearApprovals()
			}
			if approver != nil {
				approver.ClearRememberedSignings()
			}
		},
		OnRemoteChange:       notifier.NotifyRemoteChange,
		OnShareLinksChange:   notifier.NotifyShareLinksChange,
		OnThemeChange:        notifier.NotifyThemeChange,
		OnWebsiteIconsChange: notifier.NotifyWebsiteIconsChange,
		OnUpdateRequired: func(requirement app.UpdateRequirement) {
			if showUpdateRequired != nil {
				showUpdateRequired(requirement)
			}
		},
		ShowItem: func(location app.ItemLocation) {
			if showItem != nil {
				showItem(location)
			}
		},
		DescribeApp: func(bundlePath string) (string, string) {
			described := requesterResolver.DescribeApp(bundlePath)
			return described.Name, described.IconURL
		},
		AgentGrantHeads: granthead.New(getAgentGrantHeadService(statePath)),
		SignInDriver:    axfill.New(requesterResolver),
	}
	vaultService, err := app.NewService(vaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	updateService := app.NewUpdateService(vaultService, newAppUpdater())
	executablePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	helpers := getHelperPaths(filepath.Dir(executablePath))

	var wailsApp *application.App
	var window *application.WebviewWindow
	settingsWindowService := &SettingsWindowService{isUnlocked: func() bool { return vaultService.GetStatus().State == app.StateUnlocked }}
	// The browser bridge calls this from its own goroutines; AppKit aborts unless window calls run on the main thread.
	showWindow := func() {
		application.InvokeAsync(func() {
			background.SetDockIconVisible(true)
			wailsApp.Show()
			window.UnMinimise()
			window.Show()
			window.Focus()
		})
	}
	// Settings closes with the main window: Hisar then runs in the background, without a Dock icon.
	hideWindow := func() {
		settingsWindowService.Hide()
		window.Hide()
		background.SetDockIconVisible(false)
	}
	var approvalWindow *application.WebviewWindow
	approvalTouchID := touchID.CreateWindowPrompt(func() unsafe.Pointer {
		if approvalWindow == nil {
			return nil
		}
		return approvalWindow.NativeWindow()
	}, biometric.PromptSizeSmall)
	approvalPreference := app.NewApprovalPreference(vaultService)
	approver = agent.NewApprover(agent.ApproverConfig{
		Biometric:       approvalTouchID,
		CancelBiometric: approvalTouchID.CancelPrompt,
		Emit:            func(eventName string, data any) { wailsApp.Event.Emit(eventName, data) },
		ShowApprovalWindow: func(isCompact bool) {
			application.InvokeAsync(func() {
				placeApprovalWindow(approvalWindow, isCompact, 0, true)
				approvalWindow.Show()
				approvalWindow.Focus()
			})
		},
		HideApprovalWindow: func() { application.InvokeAsync(func() { approvalWindow.Hide() }) },
		ResolveRequester:   requesterResolver.Resolve,
		ReturnToApp:        reopenApp,
		GetAppIconPNG:      requesterResolver.GetIconPNG,
		HomeDirectory:      homeDirectory,
		IsCompactApproval:  approvalPreference.IsCompactSSHApproval,
		Unlocker:           app.NewApprovalUnlocker(vaultService, func() { wailsApp.Event.Emit(eventUnlocked, "approval") }),
		PhoneApprover:      app.NewPhoneApprover(vaultService),
		GetSigningRememberDuration: func() time.Duration {
			return time.Duration(approvalPreference.GetSigningRememberSeconds()) * time.Second
		},
		GetSSHRememberDuration: func() time.Duration {
			return agent.GetSSHRememberDuration(approvalPreference.GetSSHRememberSeconds())
		},
	})
	app.SetPasskeyVerifier(vaultService, approver)
	agentSessions := agentharness.NewSessions(nil)
	commandLineApprovals := clisocket.NewSessionApprovals(clisocket.DefaultApprovalLifetime, nil, agentSessions)
	commandLineBridge := app.NewCommandLineBridge(vaultService, commandLineApprovals)
	var commandLineService *commandline.Service
	commandLineServer := clisocket.NewServer(clisocket.ServerConfig{
		Handler:             commandLineBridge,
		CheckUpdateRequired: commandLineBridge.CheckUpdateRequired,
		Approver:            approver,
		AgentApprover:       approver,
		LauncherApprover:    approver,
		IdentifyApp: func(pid int) (clisocket.AppIdentity, bool) {
			return commandline.IdentifyApp(requesterResolver.Resolve(pid), requesterResolver)
		},
		PeerResolver: commandline.PeerResolver{Resolver: requesterResolver},
		Approvals:    commandLineApprovals,
		AgentHosts:   commandLineBridge,
		IsEnabled:    func() bool { return commandLineService.IsIntegrationEnabled() },
	})
	sshPeerResolver := agent.PeerResolver{
		Base:      localsocket.SystemPeerResolver{},
		Resolve:   requesterResolver.Resolve,
		ReadChain: requesterResolver.ReadChain,
		HasTerminal: func(pid int) bool {
			session, err := clisocket.SystemPeerResolver{}.GetTerminalSession(pid)
			return err == nil && session.TerminalDevice != 0
		},
		IdentifyApp: func(requester appinfo.Requester) (clisocket.AppIdentity, bool) {
			return commandline.IdentifyApp(requester, requesterResolver)
		},
		AgentHosts:    commandLineServer,
		AgentSessions: agentSessions,
	}
	agentServer = sshagent.NewServer(app.NewSSHKeySource(vaultService), approver, sshPeerResolver)
	systemNotifications := newSystemNotifications()
	agentServer.SetNoAgentKeyNotifier(func(session agentharness.Session) {
		title, text := agent.DescribeMissingAgentKey(session)
		id := fmt.Sprintf("no-agent-ssh-key-%d-%d", session.PID, session.StartTime)
		systemNotifications.show(id, title, text, func() {
			showWindow()
			wailsApp.Event.Emit("show-settings", "developer")
		})
	})
	agentServer.SetKnownHostsPaths([]string{filepath.Join(homeDirectory, ".ssh", "known_hosts"), "/etc/ssh/ssh_known_hosts"})
	socketPath := os.Getenv("HISAR_SSH_AGENT_SOCKET")
	if socketPath == "" {
		socketPath = agent.GetDefaultSocketPath(homeDirectory)
	}
	agentService := agent.NewService(agent.Config{
		HomeDirectory:      homeDirectory,
		SocketPath:         socketPath,
		Server:             agentServer,
		Approver:           approver,
		ApprovalPreference: approvalPreference,
		FitApprovalWindow: func(isCompact bool, height int) {
			application.InvokeAsync(func() { placeApprovalWindow(approvalWindow, isCompact, height, false) })
		},
	})
	agentContext, stopAgent := context.WithCancel(context.Background())
	agentStopped := make(chan struct{})

	browserHostPath := helpers.browserHost
	browserBridge := app.NewBrowserBridge(vaultService, app.BrowserBridgeConfig{
		ShowPairing: func(request *app.BrowserPairingRequest) { wailsApp.Event.Emit(eventBrowserPairing, request) },
		ShowWindow:  showWindow,
		OnUnlock:    func() { wailsApp.Event.Emit(eventUnlocked, "browser") },
		InstallHostManifests: func() []browserprotocol.BrowserInstallation {
			if _, err := os.Stat(browserHostPath); err != nil {
				return []browserprotocol.BrowserInstallation{}
			}
			return browser.InstallHostManifests(homeDirectory, browserHostPath)
		},
	})
	browserSocketPath := os.Getenv("HISAR_BROWSER_SOCKET")
	if browserSocketPath == "" {
		browserSocketPath = browser.GetDefaultSocketPath(homeDirectory)
	}
	browserServer := browser.NewServer(app.NewBrowserRequestHandler(browserBridge), localsocket.SystemPeerResolver{}, browserHostPath)
	if portable.Detect(os.Getenv, homeDirectory).Kind == portable.KindFlatpak {
		browserServer.AllowHostOutsideSandbox()
	}

	commandLineSocketPath := os.Getenv(cliprotocol.SocketEnvironment)
	if commandLineSocketPath == "" {
		commandLineSocketPath = cliprotocol.GetDefaultSocketPath(homeDirectory)
	}
	commandLineRunner := clisocket.NewRunner(commandLineServer, commandLineSocketPath)
	userPathReader := &commandline.UserPathReader{Shell: getLoginShell(), Variables: harness.ListUserVariables()}
	commandLineService = commandline.NewService(vaultService, commandline.NewInstaller(commandline.InstallerConfig{
		BundledPath:          helpers.commandLine,
		IsInstalledByPackage: helpers.isCommandLineFromPackage,
		HomeDirectory:        homeDirectory,
		GetUserPath:          userPathReader.Read,
		Shell:                getLoginShell(),
	}), commandLineRunner, commandLineApprovals, requesterResolver.DescribeApp)

	loginItemService := loginitem.NewService(vaultService, background.NewLoginItem(homeDirectory))
	harnessService := harness.NewService(harness.Config{
		HomeDirectory:   homeDirectory,
		GetUserPath:     userPathReader.Read,
		GetUserVariable: userPathReader.ReadVariable,
		OmpPluginPath:   harness.FindOmpPlugin(filepath.Dir(executablePath)),
		HisarPath:       helpers.commandLine,
	})
	inviteLinkService := app.NewInviteLinkService()
	isHiddenLaunchRequested := background.IsHiddenLaunchRequested(os.Args[1:])
	background.StartLaunchObserver()

	wailsApp = application.New(application.Options{
		Name:        "Hisar",
		Description: "End-to-end encrypted password, SSH key and secrets manager for developers",
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: getSingleInstanceID(statePath),
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if !background.IsHiddenLaunchRequested(data.Args) {
					showWindow()
				}
			},
		},
		Services: []application.Service{
			application.NewService(vaultService),
			application.NewService(&TouchIDPromptService{biometric: touchID}),
			application.NewService(&ApprovalTouchIDService{prompt: approvalTouchID}),
			application.NewService(agentService),
			application.NewService(browserBridge),
			application.NewService(updateService),
			application.NewService(commandLineService),
			application.NewService(loginItemService),
			application.NewService(inviteLinkService),
			application.NewService(harnessService),
			application.NewService(settingsWindowService),
			application.NewService(emergencyKitService),
		},
		OnShutdown: func() {
			app.ClearCopiedText(vaultService)
			commandLineRunner.Stop()
			stopAgent()
			// A pending Touch ID approval can keep a connection open; do not hold up quitting for it.
			select {
			case <-agentStopped:
			case <-time.After(2 * time.Second):
			}
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		// Accessory until the main window shows, so a hidden launch never shows a Dock icon or takes the focus.
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})
	fallbackClipboard.app, notifier.app, settingsWindowService.app = wailsApp, wailsApp, wailsApp
	// Register after NewCommandLineBridge sets the account, which notifies listeners.
	commandLineApprovals.WatchAgentSessions(func() { wailsApp.Event.Emit(eventAgentSessionsChanged, "") })

	window = wailsApp.Window.NewWithOptions(mainWindowOptions())
	// A dialog opens over the window that asked for it: the main window, or Settings when it has its own window.
	getDialogWindow := func() application.Window {
		if current := wailsApp.Window.Current(); current != nil {
			return current
		}
		return window
	}
	fileSaver.chooseSavePath = func(defaultFileName string) (string, error) {
		return wailsApp.Dialog.SaveFile().
			SetFilename(defaultFileName).
			AddFilter("PDF document", "*.pdf").
			CanCreateDirectories(true).
			AttachToWindow(getDialogWindow()).
			PromptForSingleSelection()
	}
	// No file type filter: exports often have other extensions, and the importer detects the format from the content.
	fileOpener.chooseOpenPath = func() (string, error) {
		return wailsApp.Dialog.OpenFile().
			SetTitle("Import items").
			CanChooseFiles(true).
			AttachToWindow(getDialogWindow()).
			PromptForSingleSelection()
	}
	chooseEmergencyKit = func() (string, error) {
		return wailsApp.Dialog.OpenFile().
			SetTitle("Open Emergency Kit").
			AddFilter("PDF document", "*.pdf").
			CanChooseFiles(true).
			AttachToWindow(getDialogWindow()).
			PromptForSingleSelection()
	}
	touchID.SetWindowProvider(window.NativeWindow)
	// Closing the main window keeps the SSH agent and the CLI socket running; quitting is explicit.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		hideWindow()
	})
	wailsApp.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		event.Cancel()
		showWindow()
	})
	showUpdateRequired = func(requirement app.UpdateRequirement) {
		wailsApp.Event.Emit(eventUpdateRequired, requirement)
		showWindow()
	}
	showItem = func(location app.ItemLocation) {
		wailsApp.Event.Emit(eventShowItem, location)
		showWindow()
	}
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		invite, err := app.OpenInviteLink(inviteLinkService, event.Context().URL())
		if err != nil {
			return
		}
		wailsApp.Event.Emit(eventInviteLink, invite)
		showWindow()
	})
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		background.SetAppAppearance(vaultService.GetTheme().Appearance)
		updater.Start()
		go vaultService.CheckMinimumVersion()
		// Runs after AppKit has delivered the launch event, which tells whether this is a launch at login.
		application.InvokeAsync(func() {
			if !isHiddenLaunchRequested && !background.WasLaunchedAsLoginItem() {
				showWindow()
			}
		})
		go func() {
			if err := loginitem.ApplyOnLaunch(loginItemService); err != nil {
				log.Printf("open at login: %v", err)
			}
		}()
	})
	createMenuBarItem(wailsApp, vaultService, showWindow)
	if runtime.GOOS == "darwin" {
		wailsApp.Menu.SetApplicationMenu(createApplicationMenu(hideWindow))
	}

	approvalWindow = wailsApp.Window.NewWithOptions(approvalWindowOptions())
	// Closing the approval window denies what it shows; the window is reused for the next request.
	approvalWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		approver.DenyAll()
		approvalWindow.Hide()
	})
	if runtime.GOOS == "darwin" {
		settingsWindow := wailsApp.Window.NewWithOptions(settingsWindowOptions())
		settingsWindowService.window, settingsWindowService.showMainWindow = settingsWindow, showWindow
		settingsWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			settingsWindowService.Hide()
		})
		app.AddStatusListener(vaultService, func() { settingsWindowService.followStatus(vaultService.GetStatus().State) })
	}

	go func() {
		defer close(agentStopped)
		if err := agent.Serve(agentContext, agentService); err != nil {
			log.Printf("SSH agent on %s: %v", socketPath, err)
		}
	}()
	commandline.StartIntegration(commandLineService)
	go func() {
		app.InstallBrowserHostManifest(browserBridge)
		if err := browserServer.Serve(agentContext, browserSocketPath); err != nil {
			log.Printf("browser extension bridge on %s: %v", browserSocketPath, err)
		}
	}()

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
