package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
)

const (
	// dialTimeout bounds reaching the relay.
	dialTimeout = 10 * time.Second
	// sendTimeout bounds one whole delivery: dial, TLS, authentication and
	// the transfer. Without it a relay that stops answering mid-conversation
	// holds the sending worker forever.
	sendTimeout = 60 * time.Second
)

type SMTPClient struct {
	conf        SMTPConfig
	implicitTLS bool
	tlsConfig   *tls.Config
	// timeout bounds one delivery (sendTimeout).
	timeout time.Duration
}

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	From     string `json:"from"`
	ReplyTo  string `json:"reply_to"`
	SSL      bool   `json:"ssl"`
	SiteName string `json:"siteName"`
	// InsecureSkipVerify accepts any server certificate. It exists only for
	// relays with self-signed certificates and must be set explicitly in the
	// stored platform config; certificates are verified by default.
	InsecureSkipVerify bool `json:"insecure_skip_verify"`
}

func NewSMTPClient(conf *SMTPConfig) *SMTPClient {
	if conf == nil {
		return nil
	}
	return &SMTPClient{
		conf:        *conf,
		implicitTLS: implicitTLS(conf),
		// Without implicit TLS the session upgrades with STARTTLS whenever
		// the relay offers it and stays plain otherwise, so relays without
		// TLS keep working.
		tlsConfig: &tls.Config{
			InsecureSkipVerify: conf.InsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
			ServerName:         conf.Host,
		},
		timeout: sendTimeout,
	}
}

// implicitTLS reports whether the connection starts with a TLS handshake
// (SMTPS) instead of upgrading through STARTTLS. Port 465 is implicit TLS by
// definition and the SSL flag selects it on any other port, except the
// relay and submission ports 25 and 587: they always start in plaintext, so
// honoring the flag there would only break configurations that set it to
// mean "use encryption" and have been sending through STARTTLS.
func implicitTLS(conf *SMTPConfig) bool {
	switch conf.Port {
	case 465:
		return true
	case 25, 587:
		return false
	}
	return conf.SSL
}

func (m *SMTPClient) Send(to []string, subject, body string) error {
	return m.SendContext(context.Background(), to, subject, body)
}

// SendContext delivers the message over one SMTP conversation, bounded by
// ctx and by the client's own deadline. When ctx ends the conversation the
// error matches ctx's error; when the relay outlasts the client's deadline it
// is an ordinary delivery failure.
func (m *SMTPClient) SendContext(ctx context.Context, to []string, subject, body string) error {
	deliveryCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	err := m.deliver(deliveryCtx, m.message(to, subject, body))
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	return err
}

func (m *SMTPClient) message(to []string, subject, body string) *gomail.Message {
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", m.conf.From, m.conf.SiteName)
	if m.conf.ReplyTo != "" {
		msg.SetHeader("Reply-To", m.conf.ReplyTo)
	}
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)
	return msg
}

func (m *SMTPClient) deliver(ctx context.Context, msg *gomail.Message) error {
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.conf.Host, strconv.Itoa(m.conf.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// One deadline for the whole conversation, brought forward when ctx is
	// cancelled so a blocked read or write returns at once.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	client, err := m.handshake(conn)
	if err != nil {
		return err
	}
	if err := gomail.Send(transfer{client: client}, msg); err != nil {
		return err
	}
	// The relay has accepted the message; a failed goodbye changes nothing.
	_ = client.Quit()
	return nil
}

// handshake opens the SMTP session on conn: implicit TLS or an opportunistic
// STARTTLS, then authentication when credentials are configured and the
// relay offers it.
func (m *SMTPClient) handshake(conn net.Conn) (*smtp.Client, error) {
	if m.implicitTLS {
		conn = tls.Client(conn, m.tlsConfig)
	}
	client, err := smtp.NewClient(conn, m.conf.Host)
	if err != nil {
		return nil, err
	}
	if !m.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(m.tlsConfig); err != nil {
				return nil, err
			}
		}
	}
	if auth := m.auth(client); auth != nil {
		if err := client.Auth(auth); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// auth picks the mechanism the way gomail did: CRAM-MD5 when offered, LOGIN
// when offered without PLAIN, PLAIN otherwise.
func (m *SMTPClient) auth(client *smtp.Client) smtp.Auth {
	if m.conf.User == "" {
		return nil
	}
	ok, mechanisms := client.Extension("AUTH")
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(mechanisms, "CRAM-MD5"):
		return smtp.CRAMMD5Auth(m.conf.User, m.conf.Pass)
	case strings.Contains(mechanisms, "LOGIN") && !strings.Contains(mechanisms, "PLAIN"):
		return &loginAuth{username: m.conf.User, password: m.conf.Pass, host: m.conf.Host}
	default:
		return smtp.PlainAuth("", m.conf.User, m.conf.Pass, m.conf.Host)
	}
}

// transfer runs the mail transaction of one message on an open session.
type transfer struct {
	client *smtp.Client
}

func (t transfer) Send(from string, to []string, msg io.WriterTo) error {
	if err := t.client.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := t.client.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := t.client.Data()
	if err != nil {
		return err
	}
	if _, err := msg.WriteTo(w); err != nil {
		_ = w.Close()
		return err
	}
	return w.Close()
}

// loginAuth implements the LOGIN mechanism, which net/smtp lacks. Like
// PLAIN it sends the credentials only over TLS, unless the relay advertised
// LOGIN.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		advertised := false
		for _, mechanism := range server.Auth {
			if mechanism == "LOGIN" {
				advertised = true
				break
			}
		}
		if !advertised {
			return "", nil, errors.New("smtp: unencrypted connection")
		}
	}
	if server.Name != a.host {
		return "", nil, errors.New("smtp: wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch {
	case bytes.Equal(fromServer, []byte("Username:")):
		return []byte(a.username), nil
	case bytes.Equal(fromServer, []byte("Password:")):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("smtp: unexpected server challenge: %s", fromServer)
	}
}
