package notifications

import (
	"context"
	"log/slog"
)

// EmailProvider and SmsProvider are the abstraction the blueprint's own
// "Provider abstraction" describes — Service depends on these
// interfaces only, never on a concrete email/SMS implementation, so a
// real provider can be substituted later without changing any of
// Service's, or any of orders/tracking/assignment/rescheduling's, own
// logic. Two small, focused interfaces rather than one combined
// interface, matching this project's own general preference for narrow,
// single-purpose types.
type EmailProvider interface {
	SendEmail(ctx context.Context, to, subject, body string) error
}

type SmsProvider interface {
	SendSMS(ctx context.Context, to, body string) error
}

// LogEmailProvider and LogSmsProvider are the MVP implementations —
// they never contact any external service, never require credentials of
// any kind, and satisfy every mandatory requirement this module actually
// has (an attempt is made, its content is observable, and it can fail
// safely) without adding a single new dependency to go.mod. Swapping in
// a real provider later is a pure implementation substitution behind
// the same two interfaces above.
type LogEmailProvider struct{}

func NewLogEmailProvider() LogEmailProvider { return LogEmailProvider{} }

func (LogEmailProvider) SendEmail(_ context.Context, to, subject, body string) error {
	slog.Info("email notification", "to", to, "subject", subject, "body", body)
	return nil
}

type LogSmsProvider struct{}

func NewLogSmsProvider() LogSmsProvider { return LogSmsProvider{} }

func (LogSmsProvider) SendSMS(_ context.Context, to, body string) error {
	slog.Info("sms notification", "to", to, "body", body)
	return nil
}
