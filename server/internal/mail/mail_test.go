package mail_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
)

func createTestCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "hisar-smtp-test"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		t.Fatal(err)
	}
	trustedCertificates := x509.NewCertPool()
	trustedCertificates.AddCert(certificate)
	return tls.Certificate{Certificate: [][]byte{certificateDER}, PrivateKey: privateKey}, trustedCertificates
}

func startSink(t *testing.T, config mailtest.SinkConfig) (*mailtest.Sink, string, int) {
	t.Helper()
	sink, err := mailtest.StartSink("127.0.0.1:0", config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sink.Close() })
	host, portText, _ := net.SplitHostPort(sink.Address())
	port, _ := strconv.Atoi(portText)
	return sink, host, port
}

func decodeBody(t *testing.T, data string) string {
	t.Helper()
	parsed, err := netmail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

var testMessage = mail.Message{To: "bob@example.com", Subject: "Alice\r\nBcc: eve@example.com shared a vault — with you", Body: "Line one.\nA long line " + strings.Repeat("x", 100) + "\n.\nLast line.\n"}

func TestSendOverStartTLS(t *testing.T) {
	certificate, trustedCertificates := createTestCertificate(t)
	sink, host, port := startSink(t, mailtest.SinkConfig{TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}}})
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{
		Host: host, Port: port, Username: "hisar", Password: "secret", From: "Hisar <noreply@tryhisar.com>",
		TLSConfig: &tls.Config{RootCAs: trustedCertificates},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.Send(context.Background(), testMessage); err != nil {
		t.Fatal(err)
	}
	messages := sink.ListMessages()
	if len(messages) != 1 {
		t.Fatalf("messages = %+v", messages)
	}
	received := messages[0]
	if !received.IsTLS || received.Username != "hisar" || received.From != "noreply@tryhisar.com" || len(received.Recipients) != 1 || received.Recipients[0] != "bob@example.com" {
		t.Errorf("received = %+v", received)
	}
	parsed, err := netmail.ReadMessage(strings.NewReader(received.Data))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("Bcc") != "" || parsed.Header.Get("To") != "bob@example.com" || parsed.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("headers = %v", parsed.Header)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != "AliceBcc: eve@example.com shared a vault — with you" {
		t.Errorf("subject = %q %v", subject, err)
	}
	if body := decodeBody(t, received.Data); body != strings.TrimSuffix(strings.ReplaceAll(testMessage.Body, "\n", "\r\n"), "\r\n") {
		t.Errorf("body = %q", body)
	}
}

func TestSendOverImplicitTLS(t *testing.T) {
	certificate, trustedCertificates := createTestCertificate(t)
	sink, host, port := startSink(t, mailtest.SinkConfig{TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}}, IsImplicitTLS: true})
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: host, Port: port, Username: "hisar", Password: "secret", From: "noreply@tryhisar.com", TLSConfig: &tls.Config{RootCAs: trustedCertificates}})
	if err != nil {
		t.Fatal(err)
	}
	mail.SetImplicitTLS(mailer, true)
	if err := mailer.Send(context.Background(), testMessage); err != nil {
		t.Fatal(err)
	}
	if messages := sink.ListMessages(); len(messages) != 1 || !messages[0].IsTLS {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestSendRefusesUntrustedCertificate(t *testing.T) {
	certificate, _ := createTestCertificate(t)
	sink, host, port := startSink(t, mailtest.SinkConfig{TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}}})
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: host, Port: port, Username: "hisar", Password: "secret", From: "noreply@tryhisar.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.Send(context.Background(), testMessage); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("untrusted certificate: %v", err)
	}
	if len(sink.ListMessages()) != 0 {
		t.Error("message sent over an untrusted connection")
	}
}

func TestSendRefusesPlaintextToRemoteHost(t *testing.T) {
	sink, host, port := startSink(t, mailtest.SinkConfig{})
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: host, Port: port, Username: "hisar", Password: "secret", From: "noreply@tryhisar.com"})
	if err != nil {
		t.Fatal(err)
	}
	mail.SetLoopbackCheck(mailer, func(string) bool { return false })
	if err := mailer.Send(context.Background(), testMessage); !errors.Is(err, mail.ErrNoTLS) {
		t.Errorf("plaintext to a remote host: %v", err)
	}
	if len(sink.ListMessages()) != 0 {
		t.Error("message sent without TLS")
	}
}

func TestSendAllowsPlaintextOnLoopback(t *testing.T) {
	sink, host, port := startSink(t, mailtest.SinkConfig{})
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: host, Port: port, Username: "hisar", Password: "secret", From: "noreply@tryhisar.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.Send(context.Background(), testMessage); err != nil {
		t.Fatal(err)
	}
	if messages := sink.ListMessages(); len(messages) != 1 || messages[0].IsTLS || messages[0].Username != "hisar" {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestSendRefusesInvalidRecipient(t *testing.T) {
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: "127.0.0.1", Port: 1, From: "noreply@tryhisar.com"})
	if err != nil {
		t.Fatal(err)
	}
	for _, recipient := range []string{"", "Bob <bob@example.com>", "bob@example.com\r\nBcc: eve@example.com", "not an email"} {
		if err := mailer.Send(context.Background(), mail.Message{To: recipient}); !errors.Is(err, mail.ErrInvalidRecipient) {
			t.Errorf("%q: %v", recipient, err)
		}
	}
}

func TestSendErrorLeavesTheRecipientOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := textproto.NewConn(connection)
		reader.PrintfLine("220 sink")
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch verb, _, _ := strings.Cut(line, " "); strings.ToUpper(verb) {
			case "EHLO", "HELO", "MAIL":
				reader.PrintfLine("250 ok")
			case "RCPT":
				reader.PrintfLine("550 5.1.1 <Bob@Example.com>: Recipient address rejected")
			default:
				reader.PrintfLine("221 bye")
				return
			}
		}
	}()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	mailer, err := mail.NewSMTPMailer(mail.SMTPConfig{Host: host, Port: port, From: "noreply@tryhisar.com"})
	if err != nil {
		t.Fatal(err)
	}
	err = mailer.Send(context.Background(), testMessage)
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "bob@example.com") || !strings.Contains(err.Error(), "Recipient address rejected") {
		t.Errorf("error: %v", err)
	}
}

func TestCreateSMTPMailerValidatesConfig(t *testing.T) {
	for _, config := range []mail.SMTPConfig{
		{Host: "", Port: 587, From: "noreply@tryhisar.com"},
		{Host: "smtp.example.com", Port: 0, From: "noreply@tryhisar.com"},
		{Host: "smtp.example.com", Port: 70000, From: "noreply@tryhisar.com"},
		{Host: "smtp.example.com", Port: 587, From: "not an address"},
	} {
		if _, err := mail.NewSMTPMailer(config); err == nil {
			t.Errorf("accepted %+v", config)
		}
	}
}

func TestRecorderWaitsForMessages(t *testing.T) {
	recorder := mailtest.NewRecorder()
	go func() {
		time.Sleep(10 * time.Millisecond)
		recorder.Send(context.Background(), mail.Message{To: "bob@example.com"})
	}()
	if messages := recorder.WaitForMessageCount(1, 5*time.Second); len(messages) != 1 {
		t.Fatalf("messages = %+v", messages)
	}
	if messages := recorder.WaitForMessageCount(2, 20*time.Millisecond); len(messages) != 1 {
		t.Errorf("messages after timeout = %+v", messages)
	}
}
