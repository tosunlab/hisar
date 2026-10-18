// Command hisar-browser-host is the native messaging host the browser starts; it relays to the running Hisar app.
package main

import (
	"log"
	"os"
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
	config.LaunchApp = getAppLauncher()
	if err := browser.RunHost(os.Stdin, os.Stdout, config); err != nil {
		log.Fatal(err)
	}
}
