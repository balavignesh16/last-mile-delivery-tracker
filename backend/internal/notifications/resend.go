package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// resendAPIURL is a var, not a const, purely so tests can point a
// ResendEmailProvider at an httptest.Server instead of the real Resend
// API — production code never changes it.
var resendAPIURL = "https://api.resend.com/emails"

// ResendEmailProvider sends real email via Resend's REST API
// (https://resend.com) — chosen as the one real, free-tier EmailProvider
// this project ships, behind the exact same interface LogEmailProvider
// already satisfies (see provider.go's own doc comment: "Swapping in a
// real provider later is a pure implementation substitution"). It is
// never the default — main.go only constructs one when EMAIL_PROVIDER=
// resend is explicitly set, and the API key/from address are read from
// RESEND_API_KEY/RESEND_FROM_EMAIL, never hardcoded or committed.
//
// httpClient carries its own fixed timeout, independent of ctx's own
// deadline (if any) — a slow or unreachable Resend outage must never
// stall the post-commit hook that calls this (see Service.dispatch's
// own doc comment on why that guarantee matters project-wide), and this
// is the one provider implementation where that risk is real, since
// LogEmailProvider never leaves the process at all.
type ResendEmailProvider struct {
	apiKey     string
	fromAddr   string
	httpClient *http.Client
}

func NewResendEmailProvider(apiKey, fromAddr string) ResendEmailProvider {
	return ResendEmailProvider{
		apiKey:     apiKey,
		fromAddr:   fromAddr,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func (p ResendEmailProvider) SendEmail(ctx context.Context, to, subject, body string) error {
	payload, err := json.Marshal(resendEmailRequest{
		From: p.fromAddr, To: []string{to}, Subject: subject, Text: body,
	})
	if err != nil {
		return fmt.Errorf("resend: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendAPIURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body close error is not actionable here

	if resp.StatusCode >= 300 {
		return fmt.Errorf("resend: unexpected status %d", resp.StatusCode)
	}
	return nil
}
