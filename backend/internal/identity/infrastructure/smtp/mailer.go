package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"time"

	identityapp "github.com/jfxdev/grom/backend/internal/identity/application"
	gomail "github.com/wneessen/go-mail"
)

const sendTimeout = 10 * time.Second

type TLSMode string

const (
	TLSModeStartTLS TLSMode = "starttls"
	TLSModeTLS      TLSMode = "tls"
)

type Config struct {
	Host        string
	Port        int
	FromAddress string
	TLSMode     TLSMode
	Username    string
	Password    string
}

type Mailer struct {
	config    Config
	tlsConfig *tls.Config
}

func New(config Config) (*Mailer, error) {
	return newMailer(config, nil)
}

func newMailer(config Config, tlsConfig *tls.Config) (*Mailer, error) {
	if config.Host == "" || config.Port < 1 || config.Port > 65535 || config.FromAddress == "" {
		return nil, fmt.Errorf("SMTP configuration is incomplete")
	}
	if config.TLSMode != TLSModeStartTLS && config.TLSMode != TLSModeTLS {
		return nil, fmt.Errorf("SMTP TLS mode is invalid")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("SMTP credentials are incomplete")
	}
	return &Mailer{config: config, tlsConfig: tlsConfig}, nil
}

func (m *Mailer) SendPasswordReset(ctx context.Context, input identityapp.PasswordResetEmail) error {
	message := gomail.NewMsg()
	if err := message.FromFormat("Grom Registry", m.config.FromAddress); err != nil {
		return fmt.Errorf("prepare SMTP sender: %w", err)
	}
	if err := message.To(input.Recipient); err != nil {
		return fmt.Errorf("prepare SMTP recipient: %w", err)
	}
	message.Subject("Grom — Reset your password")
	message.SetBodyString(gomail.TypeTextPlain, passwordResetText(input))
	htmlBody, err := passwordResetHTML(input)
	if err != nil {
		return fmt.Errorf("render password reset email: %w", err)
	}
	message.AddAlternativeString(gomail.TypeTextHTML, htmlBody)

	tlsConfig := m.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: m.config.Host, MinVersion: tls.VersionTLS12}
	} else {
		tlsConfig = tlsConfig.Clone()
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = m.config.Host
		}
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	options := []gomail.Option{
		gomail.WithPort(m.config.Port),
		gomail.WithTimeout(sendTimeout),
		gomail.WithTLSConfig(tlsConfig),
	}
	if m.config.TLSMode == TLSModeStartTLS {
		options = append(options, gomail.WithTLSPolicy(gomail.TLSMandatory))
	} else {
		options = append(options, gomail.WithSSL())
	}
	if m.config.Username != "" {
		options = append(options,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(m.config.Username),
			gomail.WithPassword(m.config.Password),
		)
	}
	client, err := gomail.NewClient(m.config.Host, options...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	deadline, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	if err := client.DialAndSendWithContext(deadline, message); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

func passwordResetText(input identityapp.PasswordResetEmail) string {
	return fmt.Sprintf(`Grom Registry

Reset your Grom password

Hello %s,

A Grom administrator created a password reset for your account. Choose a new password here:

%s

This link expires at %s, can be used only once, and replaces any unused reset link. If you were not expecting this message, contact your Grom administrator.
`, input.Username, input.URL, input.ExpiresAt.UTC().Format(time.RFC1123))
}

var passwordResetTemplate = template.Must(template.New("password-reset").Parse(`<!doctype html>
<html lang="en">
  <body style="margin:0;background:#090d0b;color:#f2ede3;font-family:Inter,Arial,sans-serif;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="padding:32px 16px;background:#090d0b;">
      <tr><td align="center">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;border:1px solid #a97832;border-radius:12px;background:#111512;overflow:hidden;">
          <tr><td style="padding:30px 32px 12px;color:#f2ede3;font-size:25px;font-weight:760;letter-spacing:-1px;">Grom <span style="color:#aaa39a;font-weight:560;">Registry</span></td></tr>
          <tr><td style="padding:16px 32px 32px;">
            <h1 style="margin:0 0 16px;color:#f2ede3;font-size:24px;line-height:1.25;">Reset your Grom password</h1>
            <p style="margin:0 0 18px;color:#f2ede3;line-height:1.6;">Hello {{.Username}},</p>
            <p style="margin:0 0 24px;color:#aaa39a;line-height:1.6;">A Grom administrator created a password reset for your account. This link can be used only once and expires at {{.ExpiresAt}}.</p>
            <p style="margin:0 0 24px;"><a href="{{.URL}}" style="display:inline-block;border-radius:7px;background:#91ad24;color:#111704;padding:12px 18px;font-weight:700;text-decoration:none;">Choose a new password</a></p>
            <p style="margin:0;color:#aaa39a;font-size:13px;line-height:1.6;">If the button does not work, copy this link into your browser:<br><a href="{{.URL}}" style="color:#c9df6c;word-break:break-all;">{{.URL}}</a></p>
            <p style="margin:20px 0 0;color:#aaa39a;font-size:13px;line-height:1.6;">If you were not expecting this message, contact your Grom administrator.</p>
          </td></tr>
        </table>
      </td></tr>
    </table>
  </body>
</html>`))

func passwordResetHTML(input identityapp.PasswordResetEmail) (string, error) {
	var rendered bytes.Buffer
	if err := passwordResetTemplate.Execute(&rendered, struct {
		Username  string
		URL       string
		ExpiresAt string
	}{
		Username: input.Username, URL: input.URL, ExpiresAt: input.ExpiresAt.UTC().Format(time.RFC1123),
	}); err != nil {
		return "", err
	}
	return rendered.String(), nil
}
