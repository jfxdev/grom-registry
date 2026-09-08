package application

import (
	"context"
	"time"
)

// PasswordResetMailer is the outbound port used to deliver an administrative
// password reset. Implementations must treat every field except ExpiresAt as
// secret-bearing or personally identifiable and must not log them.
type PasswordResetMailer interface {
	SendPasswordReset(context.Context, PasswordResetEmail) error
}

type PasswordResetEmail struct {
	Recipient string
	Username  string
	URL       string
	ExpiresAt time.Time
}
