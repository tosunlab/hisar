package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/tryhisar/hisar/core/appversion"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/background"
	"github.com/tryhisar/hisar/desktop/biometric"
	"github.com/tryhisar/hisar/desktop/browser"
	"github.com/tryhisar/hisar/desktop/commandline"
	"github.com/tryhisar/hisar/desktop/loginitem"
	"github.com/tryhisar/hisar/desktop/systemidle"
	"github.com/tryhisar/hisar/desktop/updater"
	"github.com/tryhisar/hisar/desktop/vault"
	"github.com/tryhisar/hisar/desktop/windowplacement"
)

//go:embed all:frontend/dist
var allAsset embed.FS

func init() {
	application.RegisterEvent[string]("locked")
	application.RegisterEvent[string]("unlocked")
	application.RegisterEvent[agent.ApprovalEvent](agent.ApprovalEventName)
	application.RegisterEvent[string](agent.ApprovalClosedEventName)
	application.RegisterEvent[vault.Theme]("theme-changed")
	application.RegisterEvent[vault.UpdateRequirement]("update-required")
	application.RegisterEvent[string]("invite-link")
}

type wailsClipboard struct{ app *application.App }

func (clipboard *wailsClipboard) SetText(text string) bool {
	return clipboard.app.Clipboard.SetText(text)
}
func (clipboard *wailsClipboard) GetText() (string, bool) { return clipboard.app.Clipboard.Text() }

type wailsLockNotifier struct{ app *application.App }

func (notifier *wailsLockNotifier) NotifyLocked() { notifier.app.Event.Emit("locked", "auto-lock") }

func (notifier *wailsLockNotifier) NotifyRemoteChange() {
	if notifier.app != nil {
		notifier.app.Event.Emit("remote-change", nil)
	}
}

func (notifier *wailsLockNotifier) NotifyShareLinksChange() {
	if notifier.app != nil {
		notifier.app.Event.Emit("share-links-changed", nil)
	}
}

// NotifyThemeChange tells every window, the approval window too, and switches the native parts to light or dark.
func (notifier *wailsLockNotifier) NotifyThemeChange(theme vault.Theme) {
	background.SetAppAppearance(theme.Appearance)
	if notifier.app != nil {
		notifier.app.Event.Emit("theme-changed", theme)
	}
}

type wailsFileSaver struct {
	chooseSavePath func(defaultFileName string) (string, error)
}

func (fileSaver *wailsFileSaver) ChooseSavePath(defaultFileName string) (string, error) {
	return fileSaver.chooseSavePath(defaultFileName)
}

type wailsFileOpener struct {
	chooseOpenPath func() (string, error)
}

func (fileOpener *wailsFileOpener) ChooseOpenPath() (string, error) {
	return fileOpener.chooseOpenPath()
}

func getDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "desktop"
	}
	if len(hostname) > 100 {
		return hostname[:100]
	}
	return hostname
}

// getSingleInstanceID lets a copy with its own HISAR_DESKTOP_STATE run next to the installed app.
func getSingleInstanceID(statePath string) string {
	if os.Getenv("HISAR_DESKTOP_STATE") == "" {
		return background.BundleID
	}
	statePathHash := sha256.Sum256([]byte(statePath))
	return background.BundleID + "." + hex.EncodeToString(statePathHash[:8])
}

// getAppVersion is the bundle's version; development builds send none, so no minimum refuses them.
func getAppVersion() string {
	version, _ := updater.Updater{}.GetVersion()
	if _, err := appversion.Parse(version); err != nil {
		return ""
	}
	return version
}

func getLoginShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/zsh"
}

const (
	fullApprovalWidth     = 480
	compactApprovalWidth  = 380
	defaultApprovalHeight = 400
	compactApprovalHeight = 120
)

// placeApprovalWindow centers the full dialog, or puts the compact card at the top right like a notification; height 0 keeps a default.
// Showing picks the display under the pointer; a resize keeps the window on its display, the compact card's top-right corner fixed.
func placeApprovalWindow(approvalWindow *application.WebviewWindow, isCompact bool, height int, isShowing bool) {
	width := fullApprovalWidth
	if isCompact {
		width = compactApprovalWidth
	}
	if height <= 0 {
		height = defaultApprovalHeight
		if isCompact {
			height = compactApprovalHeight
		}
	}
	request := windowplacement.Request{Width: float64(width), Height: float64(height), IsCompact: isCompact, IsShowing: isShowing}
	if windowplacement.PlaceWindow(approvalWindow.NativeWindow(), request) {
		return
	}
	approvalWindow.SetSize(width, height)
	approvalWindow.Center()
}

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
	clipboard := &wailsClipboard{}
	touchID := biometric.Create("Hisar Touch ID")
	lockNotifier := &wailsLockNotifier{}
	fileSaver := &wailsFileSaver{}
	fileOpener := &wailsFileOpener{}
	var agentServer *sshagent.Server
	var approver *agent.Approver
	var showUpdateRequired func(requirement vault.UpdateRequirement)
	vaultConfig := vault.Config{
		StatePath:             statePath,
		CachePath:             cachePath,
		DeviceName:            getDeviceName(),
		Platform:              runtime.GOOS + "/desktop",
		AppVersion:            getAppVersion(),
		Clipboard:             clipboard,
		LockNotifier:          lockNotifier,
		GetSystemIdleDuration: systemidle.GetIdleDuration,
		Biometric:             touchID,
		FileSaver:             fileSaver,
		FileOpener:            fileOpener,
		OnLock: func() {
			if agentServer != nil {
				agentServer.ClearAllApproval()
			}
			if approver != nil {
				approver.ClearAllRememberedSigning()
			}
		},
		OnRemoteChange:     lockNotifier.NotifyRemoteChange,
		OnShareLinksChange: lockNotifier.NotifyShareLinksChange,
		OnThemeChange:      lockNotifier.NotifyThemeChange,
		OnUpdateRequired: func(requirement vault.UpdateRequirement) {
			if showUpdateRequired != nil {
				showUpdateRequired(requirement)
			}
		},
	}
	vaultService, err := vault.CreateService(vaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	updateService := vault.CreateUpdateService(vaultService, updater.Updater{})
	executablePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	executableDirectory := filepath.Dir(executablePath)

	var app *application.App
	var window *application.WebviewWindow
	// The browser bridge calls this from its own goroutines; AppKit aborts unless window calls run on the main thread.
	showWindow := func() {
		application.InvokeAsync(func() {
			background.SetDockIconVisible(true)
			app.Show()
			window.UnMinimise()
			window.Show()
			window.Focus()
		})
	}
	hideWindow := func() {
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
	approvalPreference := vault.CreateApprovalPreference(vaultService)
	requesterResolver := appinfo.CreateSystemResolver()
	approver = agent.CreateApprover(agent.ApproverConfig{
		Biometric:       approvalTouchID,
		CancelBiometric: approvalTouchID.CancelPrompt,
		Emit:            func(eventName string, data any) { app.Event.Emit(eventName, data) },
		ShowApprovalWindow: func(isCompact bool) {
			application.InvokeAsync(func() {
				placeApprovalWindow(approvalWindow, isCompact, 0, true)
				approvalWindow.Show()
				approvalWindow.Focus()
			})
		},
		HideApprovalWindow: func() { application.InvokeAsync(func() { approvalWindow.Hide() }) },
		ResolveRequester:   requesterResolver.Resolve,
		GetAppIconPNG:      requesterResolver.GetIconPNG,
		HomeDirectory:      homeDirectory,
		IsCompactApproval:  approvalPreference.IsCompactSSHApproval,
		Unlocker:           vault.CreateApprovalUnlocker(vaultService, func() { app.Event.Emit("unlocked", "approval") }),
		PhoneApprover:      vault.CreatePhoneApprover(vaultService),
		GetSigningRememberDuration: func() time.Duration {
			return time.Duration(approvalPreference.GetSigningRememberSeconds()) * time.Second
		},
	})
	peerResolver := agent.ApplicationPeerResolver{
		Base: localsocket.SystemPeerResolver{},
		FindApplicationPath: func(pid int) (string, bool) {
			requester := requesterResolver.Resolve(pid)
			return requester.App.Path, requester.App.Path != ""
		},
	}
	agentServer = sshagent.CreateServer(vault.CreateSSHKeySource(vaultService), approver, peerResolver)
	agentServer.SetAllKnownHostsPath([]string{filepath.Join(homeDirectory, ".ssh", "known_hosts"), "/etc/ssh/ssh_known_hosts"})
	socketPath := os.Getenv("HISAR_SSH_AGENT_SOCKET")
	if socketPath == "" {
		socketPath = agent.GetDefaultSocketPath(homeDirectory)
	}
	agentService := agent.CreateService(agent.Config{
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

	browserHostPath := filepath.Join(executableDirectory, "hisar-browser-host")
	browserBridge := vault.CreateBrowserBridge(vaultService, vault.BrowserBridgeConfig{
		ShowPairing: func(request *vault.BrowserPairingRequest) { app.Event.Emit("browser-pairing", request) },
		ShowWindow:  showWindow,
		InstallAllHostManifest: func() []browserprotocol.BrowserInstallation {
			if _, err := os.Stat(browserHostPath); err != nil {
				return []browserprotocol.BrowserInstallation{}
			}
			return browser.InstallAllHostManifest(homeDirectory, browserHostPath)
		},
	})
	browserSocketPath := os.Getenv("HISAR_BROWSER_SOCKET")
	if browserSocketPath == "" {
		browserSocketPath = browser.GetDefaultSocketPath(homeDirectory)
	}
	browserServer := browser.CreateServer(vault.CreateBrowserRequestHandler(browserBridge), localsocket.SystemPeerResolver{}, browserHostPath)

	commandLineApprovals := clisocket.CreateSessionApprovals(clisocket.DefaultApprovalLifetime, nil)
	commandLineBridge := vault.CreateCommandLineBridge(vaultService, commandLineApprovals)
	var commandLineService *commandline.Service
	commandLineServer := clisocket.CreateServer(clisocket.ServerConfig{
		Handler:             commandLineBridge,
		CheckUpdateRequired: commandLineBridge.CheckUpdateRequired,
		Approver:            approver,
		AgentApprover:       approver,
		Approvals:           commandLineApprovals,
		IsEnabled:           func() bool { return commandLineService.IsIntegrationEnabled() },
	})
	commandLineSocketPath := os.Getenv(cliprotocol.SocketEnvironment)
	if commandLineSocketPath == "" {
		commandLineSocketPath = cliprotocol.GetDefaultSocketPath(homeDirectory)
	}
	commandLineRunner := clisocket.CreateRunner(commandLineServer, commandLineSocketPath)
	userPathReader := &commandline.UserPathReader{Shell: getLoginShell()}
	commandLineService = commandline.NewService(vaultService, commandline.CreateInstaller(commandline.InstallerConfig{
		BundledPath:   filepath.Join(filepath.Dir(executableDirectory), "Helpers", "hisar"),
		HomeDirectory: homeDirectory,
		GetUserPath:   userPathReader.Read,
		Shell:         getLoginShell(),
	}), commandLineRunner)

	loginItemService := loginitem.NewService(vaultService, background.CreateLoginItem(homeDirectory))
	inviteLinkService := vault.CreateInviteLinkService()
	isHiddenLaunchRequested := background.IsHiddenLaunchRequested(os.Args[1:])
	background.StartLaunchObserver()

	app = application.New(application.Options{
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
		},
		OnShutdown: func() {
			commandLineRunner.Stop()
			stopAgent()
			// A pending Touch ID approval can keep a connection open; do not hold up quitting for it.
			select {
			case <-agentStopped:
			case <-time.After(2 * time.Second):
			}
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(allAsset),
		},
		// Accessory until the main window shows, so a hidden launch never shows a Dock icon or takes the focus.
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})
	clipboard.app, lockNotifier.app = app, app

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Hisar",
		Width:     1100,
		Height:    700,
		MinWidth:  760,
		MinHeight: 480,
		Hidden:    true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(13, 14, 16),
		URL:              "/",
	})
	fileSaver.chooseSavePath = func(defaultFileName string) (string, error) {
		return app.Dialog.SaveFile().
			SetFilename(defaultFileName).
			AddFilter("PDF document", "*.pdf").
			CanCreateDirectories(true).
			AttachToWindow(window).
			PromptForSingleSelection()
	}
	// No file type filter: exports often have other extensions, and the importer detects the format from the content.
	fileOpener.chooseOpenPath = func() (string, error) {
		return app.Dialog.OpenFile().
			SetTitle("Import passwords").
			CanChooseFiles(true).
			AttachToWindow(window).
			PromptForSingleSelection()
	}
	touchID.SetWindowProvider(window.NativeWindow)
	// Closing the main window keeps the SSH agent and the CLI socket running; quitting is explicit.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		hideWindow()
	})
	app.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		event.Cancel()
		showWindow()
	})
	showUpdateRequired = func(requirement vault.UpdateRequirement) {
		app.Event.Emit("update-required", requirement)
		showWindow()
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		invite, err := vault.OpenInviteLink(inviteLinkService, event.Context().URL())
		if err != nil {
			return
		}
		app.Event.Emit("invite-link", invite)
		showWindow()
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
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
	createMenuBarItem(app, vaultService, showWindow)

	approvalWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                "ssh-approval",
		Title:               "Hisar Access Requested",
		Width:               fullApprovalWidth,
		Height:              defaultApprovalHeight,
		Hidden:              true,
		AlwaysOnTop:         true,
		DisableResize:       true,
		InitialPosition:     application.WindowCentered,
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonHidden,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 36,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(13, 14, 16),
		URL:              "/?window=ssh-approval",
	})
	// Closing the approval window denies what it shows; the window is reused for the next request.
	approvalWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		approver.DenyAll()
		approvalWindow.Hide()
	})

	go func() {
		defer close(agentStopped)
		if err := agent.Serve(agentContext, agentService); err != nil {
			log.Printf("SSH agent on %s: %v", socketPath, err)
		}
	}()
	commandline.StartIntegration(commandLineService)
	go func() {
		vault.InstallBrowserHostManifest(browserBridge)
		if err := browserServer.Serve(agentContext, browserSocketPath); err != nil {
			log.Printf("browser extension bridge on %s: %v", browserSocketPath, err)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
