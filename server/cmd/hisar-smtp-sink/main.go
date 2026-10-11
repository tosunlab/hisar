// Command hisar-smtp-sink accepts every email on a loopback port and writes it to a directory, for the end-to-end test.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/quotedprintable"
	netmail "net/mail"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
)

func main() {
	address := flag.String("addr", "127.0.0.1:2525", "loopback address to listen on")
	directory := flag.String("dir", "", "directory for the received emails (required)")
	flag.Parse()
	if *directory == "" {
		log.Fatal("-dir is required")
	}
	if err := os.MkdirAll(*directory, 0o700); err != nil {
		log.Fatal(err)
	}
	var mutex sync.Mutex
	messageCount := 0
	sink, err := mailtest.StartSink(*address, mailtest.SinkConfig{OnMessage: func(received mailtest.ReceivedMessage) {
		mutex.Lock()
		defer mutex.Unlock()
		messageCount++
		for _, recipient := range received.AllRecipient {
			path := filepath.Join(*directory, fmt.Sprintf("%04d-%s.txt", messageCount, filepath.Base(recipient)))
			if err := os.WriteFile(path, []byte(decodeMessage(received)), 0o600); err != nil {
				log.Printf("write %s: %v", path, err)
			}
		}
	}})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("hisar-smtp-sink listening on %s", sink.Address())
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	<-signalChannel
	sink.Close()
}

func decodeMessage(received mailtest.ReceivedMessage) string {
	parsed, err := netmail.ReadMessage(strings.NewReader(received.Data))
	if err != nil {
		return received.Data
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	var body io.Reader = parsed.Body
	if strings.EqualFold(parsed.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
		body = quotedprintable.NewReader(parsed.Body)
	}
	decodedBody, _ := io.ReadAll(body)
	return fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n\n%s", received.From, strings.Join(received.AllRecipient, ", "), subject, strings.ReplaceAll(string(decodedBody), "\r\n", "\n"))
}
