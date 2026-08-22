package notifications

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// twilioAPIBaseURL is a var, not a const, purely so tests can point a
// TwilioSmsProvider at an httptest.Server instead of the real Twilio
// API — production code never changes it. The account SID is part of
// the path (Twilio's own API shape), not the host, so the base is
// swapped and the path is rebuilt in endpoint() below — same technique
// resend.go uses for resendAPIURL, adapted for a path-templated API.
var twilioAPIBaseURL = "https://api.twilio.com/2010-04-01"

// TwilioSmsProvider sends real SMS via Twilio's REST API
// (https://www.twilio.com) — chosen as the one real, free-trial
// SmsProvider this project ships, behind the exact same interface
// LogSmsProvider already satisfies (see provider.go's own doc comment:
// "Swapping in a real provider later is a pure implementation
// substitution"). It is never the default — main.go only constructs
// one when SMS_PROVIDER=twilio is explicitly set, and the account
// SID/auth token/from number are read from
// TWILIO_ACCOUNT_SID/TWILIO_AUTH_TOKEN/TWILIO_FROM_NUMBER, never
// hardcoded or committed.
//
// Known trial limitation, documented honestly per this project's own
// convention (see resend.go's sandbox-sender caveat, mirrored in
// docs/notifications.md): a Twilio trial account can only send SMS to
// phone numbers that have been explicitly verified in the Twilio
// console — it cannot text an arbitrary number. This does not affect
// the implementation itself, only what a trial account can be used to
// demonstrate.
//
// httpClient carries its own fixed timeout, independent of ctx's own
// deadline (if any) — a slow or unreachable Twilio outage must never
// stall the post-commit hook that calls this (see Service.dispatch's
// own doc comment on why that guarantee matters project-wide), the
// same reasoning ResendEmailProvider's timeout already documents.
type TwilioSmsProvider struct {
	accountSID string
	authToken  string
	fromNumber string
	httpClient *http.Client
}

func NewTwilioSmsProvider(accountSID, authToken, fromNumber string) TwilioSmsProvider {
	return TwilioSmsProvider{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// endpoint builds this account's Messages resource URL against the
// current twilioAPIBaseURL — never hardcoded per-call, so tests can
// swap the base and still exercise the real path-building logic.
func (p TwilioSmsProvider) endpoint() string {
	return fmt.Sprintf("%s/Accounts/%s/Messages.json", twilioAPIBaseURL, p.accountSID)
}

func (p TwilioSmsProvider) SendSMS(ctx context.Context, to, body string) error {
	form := url.Values{}
	form.Set("To", to)
	form.Set("From", p.fromNumber)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("twilio: build request: %w", err)
	}
	req.SetBasicAuth(p.accountSID, p.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// err may embed the request URL but never the Authorization
		// header/auth token — net/http's own url.Error never includes
		// request headers, only the method and URL, so this stays safe
		// to log or return as-is.
		return fmt.Errorf("twilio: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable here

	if resp.StatusCode >= 300 {
		return fmt.Errorf("twilio: unexpected status %d", resp.StatusCode)
	}
	return nil
}
