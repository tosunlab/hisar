package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/biometric"
	"github.com/tryhisar/hisar/desktop/browser"
	"github.com/tryhisar/hisar/desktop/updater"
	"github.com/tryhisar/hisar/desktop/vault"
)

//go:embed all:frontend/dist
var allAsset embed.FS

func init() {
	application.RegisterEvent[string]("locked")
	application.RegisterEvent[agent.ApprovalEvent](agent.ApprovalEventName)
	application.RegisterEvent[string](agent.ApprovalClosedEventName)
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
	vaultConfig := vault.Config{
		StatePath:    statePath,
		CachePath:    cachePath,
		DeviceName:   getDeviceName(),
		Platform:     runtime.GOOS + "/desktop",
		Clipboard:    clipboard,
		LockNotifier: lockNotifier,
		Biometric:    touchID,
		FileSaver:    fileSaver,
		FileOpener:   fileOpener,
		OnLock: func() {
			if agentServer != nil {
				agentServer.ClearAllApproval()
			}
		},
		OnRemoteChange: lockNotifier.NotifyRemoteChange,
	}
	vaultService, err := vault.CreateService(vaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	updateService := vault.CreateUpdateService(vaultService, updater.Updater{})

	var app *application.App
	var window *application.WebviewWindow
	// The browser bridge calls this from its own goroutines; AppKit aborts unless window calls run on the main thread.
	showWindow := func() {
		application.InvokeAsync(func() {
			app.Show()
			window.UnMinimise()
			window.Show()
			window.Focus()
		})
	}
	var approvalWindow *application.WebviewWindow
	approvalTouchID := touchID.CreateWindowPrompt(func() unsafe.Pointer {
		if approvalWindow == nil {
			return nil
		}
		return approvalWindow.NativeWindow()
	}, biometric.PromptSizeSmall)
	approver := agent.CreateApprover(agent.ApproverConfig{
		Biometric:       approvalTouchID,
		CancelBiometric: approvalTouchID.CancelPrompt,
		Emit:            func(eventName string, data any) { app.Event.Emit(eventName, data) },
		ShowApprovalWindow: func() {
			application.InvokeAsync(func() {
				approvalWindow.Center()
				approvalWindow.Show()
				approvalWindow.Focus()
			})
		},
		HideApprovalWindow: func() { application.InvokeAsync(func() { approvalWindow.Hide() }) },
		FindApp: func(pid int) (appinfo.App, bool) {
			return appinfo.FindApp(pid, sshagent.GetProcessPath, sshagent.GetParentPID)
		},
	})
	agentServer = sshagent.CreateServer(vault.CreateSSHKeySource(vaultService), approver, sshagent.SystemPeerResolver{})
	agentServer.SetAllKnownHostsPath([]string{filepath.Join(homeDirectory, ".ssh", "known_hosts"), "/etc/ssh/ssh_known_hosts"})
	socketPath := os.Getenv("HISAR_SSH_AGENT_SOCKET")
	if socketPath == "" {
		socketPath = agent.GetDefaultSocketPath(homeDirectory)
	}
	agentService := agent.CreateService(agent.Config{
		HomeDirectory: homeDirectory,
		SocketPath:    socketPath,
		Server:        agentServer,
		Approver:      approver,
	})
	agentContext, stopAgent := context.WithCancel(context.Background())
	agentStopped := make(chan struct{})

	browserHostPath := ""
	if executablePath, err := os.Executable(); err == nil {
		browserHostPath = filepath.Join(filepath.Dir(executablePath), "hisar-browser-host")
	}
	browserBridge := vault.CreateBrowserBridge(vaultService, vault.BrowserBridgeConfig{
		ShowPairing: func(request *vault.BrowserPairingRequest) { app.Event.Emit("browser-pairing", request) },
		ShowWindow:  showWindow,
		InstallAllHostManifest: func() []browser.BrowserInstallation {
			if _, err := os.Stat(browserHostPath); err != nil {
				return []browser.BrowserInstallation{}
			}
			return browser.InstallAllHostManifest(homeDirectory, browserHostPath)
		},
	})
	browserSocketPath := os.Getenv("HISAR_BROWSER_SOCKET")
	if browserSocketPath == "" {
		browserSocketPath = browser.GetDefaultSocketPath(homeDirectory)
	}
	browserServer := browser.CreateServer(vault.CreateBrowserRequestHandler(browserBridge), sshagent.SystemPeerResolver{}, browserHostPath)

	app = application.New(application.Options{
		Name:        "Hisar",
		Description: "Self-hosted, end-to-end encrypted password manager",
		Services: []application.Service{
			application.NewService(vaultService),
			application.NewService(&TouchIDPromptService{biometric: touchID}),
			application.NewService(&ApprovalTouchIDService{prompt: approvalTouchID}),
			application.NewService(agentService),
			application.NewService(browserBridge),
			application.NewService(updateService),
		},
		OnShutdown: func() {
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
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	clipboard.app, lockNotifier.app = app, app

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Hisar",
		Width:     1100,
		Height:    700,
		MinWidth:  760,
		MinHeight: 480,
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
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { updater.Start() })

	approvalWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                "ssh-approval",
		Title:               "Hisar Access Requested",
		Width:               460,
		Height:              430,
		Hidden:              true,
		AlwaysOnTop:         true,
		DisableResize:       true,
		InitialPosition:     application.WindowCentered,
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
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
