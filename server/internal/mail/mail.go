// Package mail sends the server's plain-text emails (invites and share invitations) over SMTP.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Message struct {
	To      string
	Subject string
	Body    string
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

var (
	ErrNoTLS            = errors.New("mail: the SMTP server does not offer STARTTLS; refusing to send without TLS")
	ErrInvalidRecipient = errors.New("mail: invalid recipient address")
)

const implicitTLSPort = 465

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// TLSConfig overrides the TLS settings, e.g. to trust a test certificate; ServerName defaults to Host.
	TLSConfig *tls.Config
	Timeout   time.Duration
}

type SMTPMailer struct {
	config            SMTPConfig
	fromAddress       *netmail.Address
	isImplicitTLS     bool
	checkLoopbackHost func(host string) bool
}

func CreateSMTPMailer(config SMTPConfig) (*SMTPMailer, error) {
	if strings.TrimSpace(config.Host) == "" {
		return nil, errors.New("mail: SMTP host is empty")
	}
	if config.Port < 1 || config.Port > 65535 {
		return nil, fmt.Errorf("mail: invalid SMTP port %d", config.Port)
	}
	fromAddress, err := netmail.ParseAddress(config.From)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid sender %q: %w", config.From, err)
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	return &SMTPMailer{config: config, fromAddress: fromAddress, isImplicitTLS: config.Port == implicitTLSPort, checkLoopbackHost: IsLoopbackHost}, nil
}

func ParseRecipient(address string) (string, error) {
	parsed, err := netmail.ParseAddress(address)
	if err != nil || parsed.Name != "" || parsed.Address != address || strings.ContainsAny(address, "\r\n") || len(address) > 254 {
		return "", ErrInvalidRecipient
	}
	return parsed.Address, nil
}

func (m *SMTPMailer) createTLSConfig() *tls.Config {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if m.config.TLSConfig != nil {
		tlsConfig = m.config.TLSConfig.Clone()
	}
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = m.config.Host
	}
	return tlsConfig
}

func IsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (m *SMTPMailer) Send(ctx context.Context, message Message) error {
	recipient, err := ParseRecipient(message.To)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, m.config.Timeout)
	defer cancel()
	isImplicitTLS := m.isImplicitTLS
	dialer := &net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.config.Host, strconv.Itoa(m.config.Port)))
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		connection.SetDeadline(deadline)
	}
	if isImplicitTLS {
		tlsConnection := tls.Client(connection, m.createTLSConfig())
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("mail: TLS: %w", err)
		}
		connection = tlsConnection
	}
	smtpClient, err := smtp.NewClient(connection, m.config.Host)
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	defer smtpClient.Close()
	if !isImplicitTLS {
		if hasStartTLS, _ := smtpClient.Extension("STARTTLS"); hasStartTLS {
			if err := smtpClient.StartTLS(m.createTLSConfig()); err != nil {
				return fmt.Errorf("mail: STARTTLS: %w", err)
			}
		} else if !m.checkLoopbackHost(m.config.Host) {
			return ErrNoTLS
		}
	}
	if m.config.Username != "" {
		if err := smtpClient.Auth(smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)); err != nil {
			return fmt.Errorf("mail: authenticate: %w", err)
		}
	}
	if err := smtpClient.Mail(m.fromAddress.Address); err != nil {
		return fmt.Errorf("mail: sender: %w", err)
	}
	if err := smtpClient.Rcpt(recipient); err != nil {
		return fmt.Errorf("mail: recipient: %w", err)
	}
	dataWriter, err := smtpClient.Data()
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if _, err := dataWriter.Write(BuildMessage(m.fromAddress, recipient, message, time.Now())); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := dataWriter.Close(); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	return smtpClient.Quit()
}

func RemoveControlCharacter(text string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, text)
}

func BuildMessage(fromAddress *netmail.Address, recipient string, message Message, now time.Time) []byte {
	var buffer bytes.Buffer
	allRandomByte := make([]byte, 16)
	rand.Read(allRandomByte)
	domain := fromAddress.Address[strings.LastIndex(fromAddress.Address, "@")+1:]
	header := func(name, value string) { fmt.Fprintf(&buffer, "%s: %s\r\n", name, value) }
	header("From", fromAddress.String())
	header("To", recipient)
	header("Subject", mime.QEncoding.Encode("utf-8", RemoveControlCharacter(message.Subject)))
	header("Date", now.Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(allRandomByte)+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	buffer.WriteString("\r\n")
	bodyWriter := quotedprintable.NewWriter(&buffer)
	bodyWriter.Write([]byte(message.Body))
	bodyWriter.Close()
	return buffer.Bytes()
}
