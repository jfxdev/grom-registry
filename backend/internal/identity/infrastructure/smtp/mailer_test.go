package smtp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
)

func TestPasswordResetEmailUsesGromStyleAndEscapesUserInput(t *testing.T) {
	input := identityapp.PasswordResetEmail{
		Recipient: "user@example.test",
		Username:  `<script>alert("x")</script>`,
		URL:       "https://grom.example/reset-password#token=grmpr_public_secret",
		ExpiresAt: time.Date(2026, 9, 8, 12, 30, 0, 0, time.UTC),
	}
	htmlBody, err := passwordResetHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Grom", "Registry", "Reset your Grom password", "#090d0b", "#91ad24", "Choose a new password"} {
		if !strings.Contains(htmlBody, expected) {
			t.Fatalf("expected HTML email to contain %q", expected)
		}
	}
	if strings.Contains(htmlBody, input.Username) || !strings.Contains(htmlBody, "&lt;script&gt;") {
		t.Fatalf("expected username to be escaped, got %s", htmlBody)
	}
	textBody := passwordResetText(input)
	if !strings.Contains(textBody, "Grom Registry") || !strings.Contains(textBody, input.URL) {
		t.Fatalf("expected text fallback to contain brand and URL, got %s", textBody)
	}
}

func TestMailerRejectsInvalidConfigurationAndFailsWithoutLeakingInput(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected incomplete configuration to fail")
	}
	mailer, err := New(Config{Host: "127.0.0.1", Port: 1, FromAddress: "grom@example.test", TLSMode: TLSModeStartTLS})
	if err != nil {
		t.Fatal(err)
	}
	err = mailer.SendPasswordReset(context.Background(), identityapp.PasswordResetEmail{
		Recipient: "target@example.test", Username: "target",
		URL: "https://grom.example/reset-password#token=never-log-this", ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err == nil {
		t.Fatal("expected unavailable SMTP server to fail")
	}
	if strings.Contains(err.Error(), "never-log-this") {
		t.Fatalf("SMTP error leaked reset token: %v", err)
	}
}

func TestMailerSendsPasswordResetOverMandatoryTLS(t *testing.T) {
	for _, mode := range []TLSMode{TLSModeStartTLS, TLSModeTLS} {
		t.Run(string(mode), func(t *testing.T) {
			listener, clientTLS, received := startTLSSMTPServer(t, mode)
			defer func() { _ = listener.Close() }()
			port := listener.Addr().(*net.TCPAddr).Port
			mailer, err := newMailer(Config{
				Host: "127.0.0.1", Port: port, FromAddress: "grom@example.test", TLSMode: mode,
			}, clientTLS)
			if err != nil {
				t.Fatal(err)
			}
			input := identityapp.PasswordResetEmail{
				Recipient: "target@example.test", Username: "target",
				URL:       "https://grom.example/reset-password#token=grmpr_public_secret",
				ExpiresAt: time.Now().UTC().Add(30 * time.Minute),
			}
			if err := mailer.SendPasswordReset(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			message := <-received
			if message.err != nil {
				t.Fatal(message.err)
			}
			for _, expected := range []string{"Grom", "Reset your Grom password", "Choose a new password", "grmpr_public_secret"} {
				if !strings.Contains(message.data, expected) {
					t.Fatalf("expected SMTP message to contain %q", expected)
				}
			}
		})
	}
}

type capturedSMTPMessage struct {
	data string
	err  error
}

func startTLSSMTPServer(t *testing.T, mode TLSMode) (net.Listener, *tls.Config, <-chan capturedSMTPMessage) {
	t.Helper()
	certificate, roots := testCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan capturedSMTPMessage, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			received <- capturedSMTPMessage{err: err}
			return
		}
		defer func() { _ = connection.Close() }()
		if mode == TLSModeTLS {
			tlsConnection := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
			if err := tlsConnection.Handshake(); err != nil {
				received <- capturedSMTPMessage{err: err}
				return
			}
			connection = tlsConnection
		}
		data, err := serveSMTPConnection(connection, mode == TLSModeStartTLS, certificate)
		received <- capturedSMTPMessage{data: data, err: err}
	}()
	return listener, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}, received
}

func serveSMTPConnection(connection net.Conn, allowStartTLS bool, certificate tls.Certificate) (string, error) {
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	if _, err := writer.WriteString("220 localhost Grom test SMTP\r\n"); err != nil {
		return "", err
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}
	command, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(command, "EHLO ") {
		return "", fmt.Errorf("expected initial EHLO, got %q: %w", command, err)
	}
	if allowStartTLS {
		if _, err := writer.WriteString("250-localhost\r\n250-STARTTLS\r\n250 OK\r\n"); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}
		command, err = reader.ReadString('\n')
		if err != nil || !strings.EqualFold(strings.TrimSpace(command), "STARTTLS") {
			return "", fmt.Errorf("expected STARTTLS, got %q: %w", command, err)
		}
		if _, err := writer.WriteString("220 begin TLS\r\n"); err != nil {
			return "", err
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}
		tlsConnection := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		if err := tlsConnection.Handshake(); err != nil {
			return "", err
		}
		connection = tlsConnection
		reader = bufio.NewReader(connection)
		writer = bufio.NewWriter(connection)
		command, err = reader.ReadString('\n')
		if err != nil || !strings.HasPrefix(command, "EHLO ") {
			return "", fmt.Errorf("expected post-TLS EHLO, got %q: %w", command, err)
		}
	}
	if _, err := writer.WriteString("250 localhost\r\n"); err != nil {
		return "", err
	}
	if err := writer.Flush(); err != nil {
		return "", err
	}

	var message strings.Builder
	for {
		command, err = reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		switch {
		case strings.HasPrefix(command, "MAIL FROM:") || strings.HasPrefix(command, "RCPT TO:") || strings.EqualFold(strings.TrimSpace(command), "RSET") || strings.EqualFold(strings.TrimSpace(command), "NOOP"):
			if _, err := writer.WriteString("250 OK\r\n"); err != nil {
				return "", err
			}
		case strings.EqualFold(strings.TrimSpace(command), "DATA"):
			if _, err := writer.WriteString("354 send message\r\n"); err != nil {
				return "", err
			}
			if err := writer.Flush(); err != nil {
				return "", err
			}
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return "", err
				}
				if line == ".\r\n" {
					break
				}
				message.WriteString(line)
			}
			if _, err := writer.WriteString("250 accepted\r\n"); err != nil {
				return "", err
			}
		case strings.EqualFold(strings.TrimSpace(command), "QUIT"):
			_, _ = writer.WriteString("221 goodbye\r\n")
			_ = writer.Flush()
			return message.String(), nil
		default:
			return "", fmt.Errorf("unexpected SMTP command %q", command)
		}
		if err := writer.Flush(); err != nil {
			return "", err
		}
	}
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate := tls.Certificate{Certificate: [][]byte{certificateDER}, PrivateKey: privateKey}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}))
	return certificate, roots
}
