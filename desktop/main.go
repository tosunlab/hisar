package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/tryhisar/hisar/desktop/biometric"
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

type wailsFileSaver struct {
	chooseSavePath func(defaultFileName string) (string, error)
}

func (fileSaver *wailsFileSaver) ChooseSavePath(defaultFileName string) (string, error) {
	return fileSaver.chooseSavePath(defaultFileName)
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
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	clipboard := &wailsClipboard{}
	touchID := biometric.Create("Hisar Touch ID")
	lockNotifier := &wailsLockNotifier{}
	fileSaver := &wailsFileSaver{}
	var agentServer *sshagent.Server
	vaultConfig := vault.Config{
		StatePath:    statePath,
		DeviceName:   getDeviceName(),
		Platform:     runtime.GOOS + "/desktop",
		Clipboard:    clipboard,
		LockNotifier: lockNotifier,
		Biometric:    touchID,
		FileSaver:    fileSaver,
		OnLock: func() {
			if agentServer != nil {
				agentServer.ClearAllApproval()
			}
		},
	}
	vaultService, err := vault.CreateService(vaultConfig)
	if err != nil {
		log.Fatal(err)
	}

	var app *application.App
	var window *application.WebviewWindow
	approver := agent.CreateApprover(agent.ApproverConfig{
		Biometric: touchID,
		Emit:      func(eventName string, data any) { app.Event.Emit(eventName, data) },
		ShowWindow: func() {
			app.Show()
			window.UnMinimise()
			window.Show()
			window.Focus()
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

	app = application.New(application.Options{
		Name:        "Hisar",
		Description: "Self-hosted, end-to-end encrypted password manager",
		Services: []application.Service{
			application.NewService(vaultService),
			application.NewService(&TouchIDPromptService{biometric: touchID}),
			application.NewService(agentService),
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
	touchID.SetWindowProvider(window.NativeWindow)

	go func() {
		defer close(agentStopped)
		if err := agent.Serve(agentContext, agentService); err != nil {
			log.Printf("SSH agent on %s: %v", socketPath, err)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
