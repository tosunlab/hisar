// Package mailtest has a recording Mailer and a small SMTP sink for tests.
package mailtest

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/server/internal/mail"
)

type Recorder struct {
	mutex      sync.Mutex
	allMessage []mail.Message
	changed    chan struct{}
	SendError  error
}

func CreateRecorder() *Recorder {
	return &Recorder{changed: make(chan struct{})}
}

func (recorder *Recorder) Send(_ context.Context, message mail.Message) error {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	if recorder.SendError != nil {
		return recorder.SendError
	}
	recorder.allMessage = append(recorder.allMessage, message)
	close(recorder.changed)
	recorder.changed = make(chan struct{})
	return nil
}

func (recorder *Recorder) ListAllMessage() []mail.Message {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	return append([]mail.Message(nil), recorder.allMessage...)
}

// WaitForMessageCount returns the messages once there are at least count of them, or what arrived before the timeout.
func (recorder *Recorder) WaitForMessageCount(count int, timeout time.Duration) []mail.Message {
	deadline := time.After(timeout)
	for {
		recorder.mutex.Lock()
		allMessage, changed := append([]mail.Message(nil), recorder.allMessage...), recorder.changed
		recorder.mutex.Unlock()
		if len(allMessage) >= count {
			return allMessage
		}
		select {
		case <-changed:
		case <-deadline:
			return allMessage
		}
	}
}

type ReceivedMessage struct {
	From         string
	AllRecipient []string
	Username     string
	IsTLS        bool
	Data         string
}

type SinkConfig struct {
	// TLSConfig enables STARTTLS, or implicit TLS with IsImplicitTLS.
	TLSConfig     *tls.Config
	IsImplicitTLS bool
	OnMessage     func(ReceivedMessage)
}

// Sink accepts every message on a local port; it implements only what the server's SMTP client uses.
type Sink struct {
	listener   net.Listener
	config     SinkConfig
	mutex      sync.Mutex
	allMessage []ReceivedMessage
	waitGroup  sync.WaitGroup
}

func StartSink(address string, config SinkConfig) (*Sink, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	if config.IsImplicitTLS {
		if config.TLSConfig == nil {
			listener.Close()
			return nil, errors.New("mailtest: implicit TLS needs a TLS config")
		}
		listener = tls.NewListener(listener, config.TLSConfig)
	}
	sink := &Sink{listener: listener, config: config}
	sink.waitGroup.Add(1)
	go sink.acceptAll()
	return sink, nil
}

func (sink *Sink) Address() string { return sink.listener.Addr().String() }

func (sink *Sink) Close() error {
	err := sink.listener.Close()
	sink.waitGroup.Wait()
	return err
}

func (sink *Sink) ListAllMessage() []ReceivedMessage {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()
	return append([]ReceivedMessage(nil), sink.allMessage...)
}

func (sink *Sink) acceptAll() {
	defer sink.waitGroup.Done()
	for {
		connection, err := sink.listener.Accept()
		if err != nil {
			return
		}
		sink.waitGroup.Add(1)
		go func() {
			defer sink.waitGroup.Done()
			defer connection.Close()
			connection.SetDeadline(time.Now().Add(time.Minute))
			_, isTLS := connection.(*tls.Conn)
			sink.serve(connection, isTLS, true)
		}()
	}
}

func (sink *Sink) serve(connection net.Conn, isTLS, isGreeting bool) {
	reader := textproto.NewReader(bufio.NewReader(connection))
	writer := textproto.NewWriter(bufio.NewWriter(connection))
	reply := func(line string) bool { return writer.PrintfLine("%s", line) == nil }
	if isGreeting && !reply("220 hisar-smtp-sink ready") {
		return
	}
	var current ReceivedMessage
	var username string
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}
		verb, argument, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			allExtension := []string{"250-hisar-smtp-sink", "250-8BITMIME", "250-AUTH PLAIN"}
			if sink.config.TLSConfig != nil && !isTLS {
				allExtension = append(allExtension, "250-STARTTLS")
			}
			allExtension = append(allExtension, "250 SMTPUTF8")
			for _, extension := range allExtension {
				if !reply(extension) {
					return
				}
			}
		case "STARTTLS":
			if sink.config.TLSConfig == nil || isTLS {
				reply("502 not available")
				continue
			}
			if !reply("220 go ahead") {
				return
			}
			tlsConnection := tls.Server(connection, sink.config.TLSConfig)
			if tlsConnection.Handshake() != nil {
				return
			}
			sink.serve(tlsConnection, true, false)
			return
		case "AUTH":
			mechanism, initialResponse, _ := strings.Cut(argument, " ")
			if !strings.EqualFold(mechanism, "PLAIN") {
				reply("504 unsupported mechanism")
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(initialResponse)
			allPart := strings.Split(string(decoded), "\x00")
			if err != nil || len(allPart) != 3 {
				reply("535 invalid credentials")
				continue
			}
			username = allPart[1]
			reply("235 authenticated")
		case "MAIL":
			current = ReceivedMessage{From: extractPath(argument), Username: username, IsTLS: isTLS}
			reply("250 ok")
		case "RCPT":
			current.AllRecipient = append(current.AllRecipient, extractPath(argument))
			reply("250 ok")
		case "DATA":
			if !reply("354 end with <CRLF>.<CRLF>") {
				return
			}
			allLine, err := reader.ReadDotLines()
			if err != nil {
				return
			}
			current.Data = strings.Join(allLine, "\r\n")
			sink.mutex.Lock()
			sink.allMessage = append(sink.allMessage, current)
			sink.mutex.Unlock()
			if sink.config.OnMessage != nil {
				sink.config.OnMessage(current)
			}
			reply("250 queued")
		case "RSET", "NOOP":
			reply("250 ok")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func extractPath(argument string) string {
	start, end := strings.Index(argument, "<"), strings.Index(argument, ">")
	if start < 0 || end < start {
		return ""
	}
	return argument[start+1 : end]
}
