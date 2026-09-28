package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestEmailSend(t *testing.T) {
	t.Skipf("Skip TestEmailSend")
	config := &SMTPConfig{
		Host:     "smtp.mail.me.com",
		Port:     587,
		User:     "support@ppanel.dev",
		Pass:     "password",
		From:     "support@ppanel.dev",
		SSL:      true,
		SiteName: "",
	}
	address := []string{"tension@sparkdance.dev"}
	subject := "test"
	body := "test"
	email := NewSMTPClient(config)
	err := email.Send(address, subject, body)
	if err != nil {
		t.Errorf("send email error: %v", err)
	}
}

func TestSMTPClientVerifiesCertificatesByDefault(t *testing.T) {
	client := NewSMTPClient(&SMTPConfig{Host: "smtp.example.test", Port: 587})
	tlsConfig := client.dailer.TLSConfig
	if tlsConfig == nil || tlsConfig.InsecureSkipVerify {
		t.Fatalf("TLS config = %+v, want certificate verification", tlsConfig)
	}
	if tlsConfig.ServerName != "smtp.example.test" {
		t.Fatalf("ServerName = %q, want the relay host", tlsConfig.ServerName)
	}
}

func TestSMTPSenderHonorsExplicitVerificationOptOut(t *testing.T) {
	sender, err := NewSender("smtp", `{"host":"smtp.example.test","port":587,"insecure_skip_verify":true}`, "Example")
	if err != nil {
		t.Fatal(err)
	}
	if !sender.(*SMTPClient).dailer.TLSConfig.InsecureSkipVerify {
		t.Fatal("insecure_skip_verify in the stored config was ignored")
	}
}

func TestSMTPClientImplicitTLSSelection(t *testing.T) {
	for _, tt := range []struct {
		port int
		ssl  bool
		want bool
	}{
		// 465 is SMTPS by definition, whatever the flag says.
		{port: 465, ssl: false, want: true},
		{port: 465, ssl: true, want: true},
		// Elsewhere the flag selects implicit TLS...
		{port: 2465, ssl: true, want: true},
		{port: 2465, ssl: false, want: false},
		// ...except on the relay and submission ports, which start plain.
		{port: 587, ssl: true, want: false},
		{port: 25, ssl: true, want: false},
	} {
		client := NewSMTPClient(&SMTPConfig{Host: "smtp.example.test", Port: tt.port, SSL: tt.ssl})
		if client.dailer.SSL != tt.want {
			t.Errorf("port %d ssl=%v: implicit TLS = %v, want %v", tt.port, tt.ssl, client.dailer.SSL, tt.want)
		}
	}
}

// A relay presenting a certificate nobody vouches for must be refused, both
// when the session upgrades through STARTTLS and when it starts with TLS.
func TestSMTPClientRejectsUntrustedCertificate(t *testing.T) {
	cert := selfSignedCertificate(t)
	for name, implicit := range map[string]bool{"starttls": false, "implicit tls": true} {
		t.Run(name, func(t *testing.T) {
			addr := startFakeSMTP(t, &cert, implicit)
			client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: addr.Port, SSL: implicit, From: "panel@example.test"})

			err := client.Send([]string{"user@example.test"}, "subject", "body")

			var verifyErr *tls.CertificateVerificationError
			if !errors.As(err, &verifyErr) {
				t.Fatalf("send error = %v, want a certificate verification failure", err)
			}
		})
	}
}

func TestSMTPClientOptOutAcceptsSelfSignedRelay(t *testing.T) {
	cert := selfSignedCertificate(t)
	addr := startFakeSMTP(t, &cert, false)
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: addr.Port, From: "panel@example.test", InsecureSkipVerify: true})

	if err := client.Send([]string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send through self-signed relay with explicit opt-out: %v", err)
	}
}

// Relays without TLS keep working: STARTTLS stays opportunistic.
func TestSMTPClientSendsThroughPlainRelay(t *testing.T) {
	addr := startFakeSMTP(t, nil, false)
	client := NewSMTPClient(&SMTPConfig{Host: "127.0.0.1", Port: addr.Port, From: "panel@example.test"})

	if err := client.Send([]string{"user@example.test"}, "subject", "body"); err != nil {
		t.Fatalf("send through plain relay: %v", err)
	}
}

func selfSignedCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "self-signed relay"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// startFakeSMTP serves one SMTP session. With a certificate it either offers
// STARTTLS or, when implicit, starts the session with the TLS handshake.
func startFakeSMTP(t *testing.T, cert *tls.Certificate, implicit bool) *net.TCPAddr {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var tlsConfig *tls.Config
	if cert != nil {
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{*cert}}
	}
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// A client that never speaks must not hang the test.
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if implicit {
			tlsConn := tls.Server(conn, tlsConfig)
			if tlsConn.Handshake() != nil {
				return
			}
			conn = tlsConn
			tlsConfig = nil
		}
		serveSMTP(conn, tlsConfig)
	}()
	return listener.Addr().(*net.TCPAddr)
}

// serveSMTP speaks just enough SMTP for one message; a non-nil starttls
// config is offered through the STARTTLS extension.
func serveSMTP(conn net.Conn, starttls *tls.Config) {
	reader := bufio.NewReader(conn)
	reply := func(format string, args ...any) { _, _ = fmt.Fprintf(conn, format+"\r\n", args...) }
	reply("220 fake ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			if starttls != nil {
				reply("250-fake")
				reply("250 STARTTLS")
			} else {
				reply("250 fake")
			}
		case command == "STARTTLS" && starttls != nil:
			reply("220 ready")
			tlsConn := tls.Server(conn, starttls)
			if tlsConn.Handshake() != nil {
				return
			}
			conn, reader, starttls = tlsConn, bufio.NewReader(tlsConn), nil
		case strings.HasPrefix(command, "MAIL FROM"), strings.HasPrefix(command, "RCPT TO"),
			command == "RSET", command == "NOOP":
			reply("250 ok")
		case command == "DATA":
			reply("354 go ahead")
			for {
				body, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if body == ".\r\n" {
					break
				}
			}
			reply("250 queued")
		case command == "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 unsupported")
		}
	}
}
