package mail

import (
	"crypto/tls"

	"gopkg.in/gomail.v2"
)

type SMTPClient struct {
	conf   SMTPConfig
	dailer *gomail.Dialer
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
	dailer := gomail.NewDialer(conf.Host, conf.Port, conf.User, conf.Pass)
	dailer.SSL = implicitTLS(conf)
	// Without SSL the dialer upgrades with STARTTLS whenever the relay offers
	// it and stays plain otherwise, so relays without TLS keep working.
	dailer.TLSConfig = &tls.Config{
		InsecureSkipVerify: conf.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
		ServerName:         conf.Host,
	}

	return &SMTPClient{conf: *conf, dailer: dailer}
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
	msg := gomail.NewMessage()
	msg.SetAddressHeader("From", m.conf.From, m.conf.SiteName)
	if m.conf.ReplyTo != "" {
		msg.SetHeader("Reply-To", m.conf.ReplyTo)
	}
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)
	return m.dailer.DialAndSend(msg)
}
