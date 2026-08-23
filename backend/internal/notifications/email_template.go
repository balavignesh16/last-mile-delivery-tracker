package notifications

import (
	"html"
	"strings"
)

// statusVisual is the badge color pairing an event's HTML status pill
// uses below — the exact same Tailwind color pairs
// frontend/src/components/order-status.ts's STATUS_STYLE already uses
// for the matching OrderStatus, so the email and the web app never
// disagree about what "delivered" or "failed" looks like. AGENT_ASSIGNED
// reuses ASSIGNED's pair and ORDER_CREATED reuses CREATED's, matching
// notification.go's own comment on why those two event names differ
// from their tracking.Status counterparts.
type statusVisual struct {
	bg, fg string
}

var eventVisual = map[EventType]statusVisual{
	EventOrderCreated:   {bg: "#f1f5f9", fg: "#334155"},
	EventAgentAssigned:  {bg: "#dbeafe", fg: "#1e40af"},
	EventPickedUp:       {bg: "#dbeafe", fg: "#1e40af"},
	EventInTransit:      {bg: "#e0e7ff", fg: "#3730a3"},
	EventOutForDelivery: {bg: "#fef3c7", fg: "#92400e"},
	EventDelivered:      {bg: "#d1fae5", fg: "#065f46"},
	EventFailed:         {bg: "#fee2e2", fg: "#991b1b"},
	EventRescheduled:    {bg: "#fef3c7", fg: "#92400e"},
}

// buildHTMLBody renders the exact same information buildContent's plain
// text Body already carries — status, order id, and any event-specific
// detail lines — as a branded HTML email. Table-based layout with only
// inline styles, no external stylesheet/font/image and no JavaScript:
// the deliverability-safe subset every major email client (Gmail,
// Outlook, Apple Mail) renders consistently.
//
// Every dynamic value is passed through html.EscapeString before
// insertion. That matters specifically for extras: failure/reschedule
// reason text ultimately originates from a customer- or admin-supplied
// string recorded in order_tracking_events.metadata, not a trusted
// constant, so this is the one place in the codebase that renders
// untrusted text as HTML and must escape it. buildContent's plain-text
// Body needs no such escaping, and SMS never renders HTML at all.
func buildHTMLBody(event EventType, statusLabel, headline, orderID string, extras []string) string {
	visual := eventVisual[event]

	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0;padding:0;background-color:#eef3fa;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#eef3fa;padding:32px 16px;"><tr><td align="center">`)
	b.WriteString(`<table role="presentation" width="560" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%;background-color:#ffffff;border-radius:8px;overflow:hidden;border:1px solid #d9e4f3;">`)

	// Brand header.
	b.WriteString(`<tr><td style="background-color:#0b1f3a;padding:20px 32px;">`)
	b.WriteString(`<span style="color:#ffffff;font-size:16px;font-weight:600;letter-spacing:0.02em;">Last-Mile Delivery Tracker</span>`)
	b.WriteString(`</td></tr>`)

	// Status pill + headline + detail box.
	b.WriteString(`<tr><td style="padding:32px;">`)
	b.WriteString(`<span style="display:inline-block;background-color:` + visual.bg + `;color:` + visual.fg + `;font-size:12px;font-weight:600;letter-spacing:0.03em;text-transform:uppercase;padding:4px 10px;border-radius:999px;">`)
	b.WriteString(html.EscapeString(statusLabel))
	b.WriteString(`</span>`)
	b.WriteString(`<h1 style="margin:16px 0 8px;font-size:20px;line-height:1.4;color:#16304f;font-weight:600;">`)
	b.WriteString(html.EscapeString(headline))
	b.WriteString(`</h1>`)

	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f8fafc;border-radius:6px;border:1px solid #e2e8f0;margin-top:8px;"><tr><td style="padding:16px 20px;">`)
	b.WriteString(`<p style="margin:0;font-size:11px;color:#64748b;text-transform:uppercase;letter-spacing:0.03em;">Order ID</p>`)
	b.WriteString(`<p style="margin:4px 0 0;font-size:14px;color:#16304f;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;word-break:break-all;">`)
	b.WriteString(html.EscapeString(orderID))
	b.WriteString(`</p>`)
	for _, e := range extras {
		b.WriteString(`<p style="margin:12px 0 0;font-size:14px;color:#334155;">`)
		b.WriteString(html.EscapeString(e))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</td></tr></table>`)
	b.WriteString(`</td></tr>`)

	// Footer.
	b.WriteString(`<tr><td style="padding:20px 32px;background-color:#f8fafc;border-top:1px solid #e2e8f0;">`)
	b.WriteString(`<p style="margin:0;font-size:12px;color:#94a3b8;">This is an automated notification — please don't reply to this email.</p>`)
	b.WriteString(`</td></tr>`)

	b.WriteString(`</table></td></tr></table></body></html>`)

	return b.String()
}
