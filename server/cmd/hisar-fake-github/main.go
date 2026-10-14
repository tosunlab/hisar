// Command hisar-fake-github serves GitHub Releases with one published and one unpublished desktop release, for the end-to-end test.
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/server/internal/desktopupdate/fakegithub"
)

func main() {
	address := flag.String("addr", "127.0.0.1:18092", "loopback address to listen on")
	token := flag.String("token", "", "bearer token the API requires (empty: none)")
	flag.Parse()
	fake := fakegithub.New(fakegithub.DefaultRepository, *token)
	fake.StorageURL = "http://" + *address + "/storage"
	publishedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fake.AddRelease(fakegithub.Release{TagName: "cli-1.0.0", Assets: []fakegithub.Asset{{Name: "SHA256SUMS", Content: []byte("sums")}}})
	fake.AddRelease(fakegithub.NewDesktopRelease(fakegithub.ReleaseConfig{
		Version: "0.2.0", BuildNumber: 2000, Archive: []byte("end-to-end archive of 0.2.0"), ReleaseNotes: "Published release", PublishedAt: publishedAt,
	}))
	fake.AddRelease(fakegithub.NewDesktopRelease(fakegithub.ReleaseConfig{
		Version: "0.3.0", BuildNumber: 3000, Archive: []byte("end-to-end archive of 0.3.0"), ReleaseNotes: "Not published yet", PublishedAt: publishedAt, IsUnpublished: true,
	}))
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", fake.ServeAPI)
	mux.Handle("/storage/", http.StripPrefix("/storage", http.HandlerFunc(fake.ServeStorage)))
	server := &http.Server{Addr: *address, Handler: mux}
	go func() {
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
		<-signalChannel
		server.Close()
	}()
	log.Printf("hisar-fake-github listening on %s", *address)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
