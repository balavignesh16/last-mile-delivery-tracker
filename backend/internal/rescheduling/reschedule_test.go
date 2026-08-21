package rescheduling

import (
	"errors"
	"testing"
	"time"
)

// --- ParseRequestedDate ---

func TestParseRequestedDate_ValidFutureDate(t *testing.T) {
	got, err := ParseRequestedDate("2026-09-01")
	if err != nil {
		t.Fatalf("ParseRequestedDate() error: %v", err)
	}
	if got.Format(dateLayout) != "2026-09-01" {
		t.Errorf("parsed date = %v, want 2026-09-01", got)
	}
}

func TestParseRequestedDate_Empty(t *testing.T) {
	_, err := ParseRequestedDate("")
	if !errors.Is(err, ErrMissingRequestedDate) {
		t.Errorf("error = %v, want ErrMissingRequestedDate", err)
	}
}

func TestParseRequestedDate_Malformed(t *testing.T) {
	cases := []string{
		"not-a-date",
		"2026/09/01",
		"09-01-2026",
		"2026-13-01",           // invalid month
		"2026-02-30",           // invalid day for February
		"2026-09-01T00:00:00Z", // timestamp, not a date
		"2026-9-1",             // missing zero-padding
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := ParseRequestedDate(tc)
			if !errors.Is(err, ErrInvalidRequestedDate) {
				t.Errorf("ParseRequestedDate(%q) error = %v, want ErrInvalidRequestedDate", tc, err)
			}
		})
	}
}

func TestParseRequestedDate_LeapDayAccepted(t *testing.T) {
	got, err := ParseRequestedDate("2028-02-29")
	if err != nil {
		t.Fatalf("ParseRequestedDate() error: %v", err)
	}
	if got.Format(dateLayout) != "2028-02-29" {
		t.Errorf("parsed date = %v, want 2028-02-29", got)
	}
}

// --- ValidateRescheduleDate ---

func TestValidateRescheduleDate_FutureDateAccepted(t *testing.T) {
	today := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	future := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := ValidateRescheduleDate(future, today); err != nil {
		t.Errorf("ValidateRescheduleDate() error = %v, want nil", err)
	}
}

func TestValidateRescheduleDate_TodayAccepted(t *testing.T) {
	// Same calendar date, but "today" carries a later time-of-day than
	// midnight — same-day rescheduling must still be accepted, proving
	// the comparison is date-only, never time-of-day.
	today := time.Date(2026, 8, 21, 23, 59, 59, 0, time.UTC)
	requested := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if err := ValidateRescheduleDate(requested, today); err != nil {
		t.Errorf("ValidateRescheduleDate() error = %v, want nil (same-day allowed)", err)
	}
}

func TestValidateRescheduleDate_YesterdayRejected(t *testing.T) {
	today := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 8, 20, 23, 59, 59, 0, time.UTC)
	err := ValidateRescheduleDate(yesterday, today)
	if !errors.Is(err, ErrPastRequestedDate) {
		t.Errorf("error = %v, want ErrPastRequestedDate", err)
	}
}

func TestValidateRescheduleDate_ClearlyPastDateRejected(t *testing.T) {
	today := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	err := ValidateRescheduleDate(past, today)
	if !errors.Is(err, ErrPastRequestedDate) {
		t.Errorf("error = %v, want ErrPastRequestedDate", err)
	}
}

func TestValidateRescheduleDate_YearBoundary(t *testing.T) {
	today := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	nextYear := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := ValidateRescheduleDate(nextYear, today); err != nil {
		t.Errorf("ValidateRescheduleDate() error = %v, want nil", err)
	}
	lastYear := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := ValidateRescheduleDate(lastYear, today); !errors.Is(err, ErrPastRequestedDate) {
		t.Errorf("error = %v, want ErrPastRequestedDate", err)
	}
}
