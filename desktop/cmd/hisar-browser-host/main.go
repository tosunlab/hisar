// Command hisar-browser-host is the native messaging host the browser starts; it relays to the running Hisar app.
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/tryhisar/hisar/desktop/browser"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("hisar-browser-host: ")
	socketPath := os.Getenv("HISAR_BROWSER_SOCKET")
	if socketPath == "" {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		socketPath = browser.GetDefaultSocketPath(homeDirectory)
	}
	extensionOrigin := ""
	if len(os.Args) > 1 {
		extensionOrigin = os.Args[1]
	}
	config := browser.HostConfig{SocketPath: socketPath, ExtensionOrigin: extensionOrigin, RetryFor: 10 * time.Second}
	if runtime.GOOS == "darwin" {
		config.LaunchApp = launchHisar
	}
	if err := browser.RunHost(os.Stdin, os.Stdout, config); err != nil {
		log.Fatal(err)
	}
}

// launchHisar opens the app bundle this host ships in, or the installed one, without bringing it to the front.
func launchHisar() error {
	executablePath, err := os.Executable()
	if err == nil {
		bundlePath := filepath.Dir(filepath.Dir(filepath.Dir(executablePath)))
		if filepath.Ext(bundlePath) == ".app" {
			return exec.Command("/usr/bin/open", "-g", bundlePath).Run()
		}
	}
	return exec.Command("/usr/bin/open", "-g", "-b", "com.tryhisar.hisar").Run()
}
