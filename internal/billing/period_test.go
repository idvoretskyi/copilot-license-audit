// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/idvoretskyi/copilot-license-audit/internal/billing"
)

// TestPreviousPeriod covers ordinary months, the January -> December/prior-
// year rollover, and December itself (should not roll over).
func TestPreviousPeriod(t *testing.T) {
	tests := []struct {
		year, month         int
		wantYear, wantMonth int
	}{
		{2026, 7, 2026, 6},  // ordinary month
		{2026, 1, 2025, 12}, // January rollover
		{2026, 12, 2026, 11},
		{2024, 3, 2024, 2},
	}
	for _, tt := range tests {
		gotYear, gotMonth := billing.PreviousPeriod(tt.year, tt.month)
		if gotYear != tt.wantYear || gotMonth != tt.wantMonth {
			t.Errorf("PreviousPeriod(%d, %d) = (%d, %d), want (%d, %d)",
				tt.year, tt.month, gotYear, gotMonth, tt.wantYear, tt.wantMonth)
		}
	}
}

// TestGetPeriodComparison verifies all 6 requests (summary/premium/credits x
// previous+current) hit the right endpoints with the right year/month, that
// day is never sent (period reports are always month-level), and that
// results land in the correct Previous/Current field.
func TestGetPeriodComparison(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		requests = append(requests, fmt.Sprintf("%s year=%s month=%s day=%s",
			r.URL.Path, q.Get("year"), q.Get("month"), q.Get("day")))

		year, month := q.Get("year"), q.Get("month")
		switch r.URL.Path {
		case "/enterprises/test-ent/settings/billing/usage/summary":
			fmt.Fprintf(w, `{"enterprise":"test-ent","timePeriod":{"year":%s,"month":%s},"usageItems":[{"product":"copilot","sku":"copilot_enterprise","netAmount":100.0}]}`, year, month)
		case "/enterprises/test-ent/settings/billing/premium_request/usage":
			fmt.Fprintf(w, `{"enterprise":"test-ent","timePeriod":{"year":%s,"month":%s},"usageItems":[]}`, year, month)
		case "/enterprises/test-ent/settings/billing/ai_credit/usage":
			fmt.Fprintf(w, `{"enterprise":"test-ent","timePeriod":{"year":%s,"month":%s},"usageItems":[]}`, year, month)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	now := time.Date(2026, time.July, 7, 12, 0, 0, 0, time.UTC)
	pc, err := billing.GetPeriodComparison(c, "test-ent", billing.UsageFilter{}, now)
	if err != nil {
		t.Fatalf("GetPeriodComparison error: %v", err)
	}

	if pc.Enterprise != "test-ent" {
		t.Errorf("Enterprise = %q, want test-ent", pc.Enterprise)
	}
	if !pc.GeneratedAt.Equal(now) {
		t.Errorf("GeneratedAt = %v, want %v", pc.GeneratedAt, now)
	}
	if pc.Previous.Year != 2026 || pc.Previous.Month != 6 {
		t.Errorf("Previous period = %d-%d, want 2026-6", pc.Previous.Year, pc.Previous.Month)
	}
	if pc.Current.Year != 2026 || pc.Current.Month != 7 {
		t.Errorf("Current period = %d-%d, want 2026-7", pc.Current.Year, pc.Current.Month)
	}
	if pc.Previous.Summary.UsageItems[0].NetAmount != 100.0 {
		t.Errorf("Previous.Summary NetAmount = %v, want 100.0", pc.Previous.Summary.UsageItems[0].NetAmount)
	}

	if len(requests) != 6 {
		t.Fatalf("made %d requests, want 6 (summary+premium+credits x 2 periods): %v", len(requests), requests)
	}
	for _, req := range requests {
		if !strings.HasSuffix(req, "day=") {
			t.Errorf("request %q included a day param; billing report is always month-level", req)
		}
	}
}

// TestGetPeriodComparison_filterForwarded verifies Organization/Product/
// CostCenterID are forwarded to every one of the 6 requests.
func TestGetPeriodComparison_filterForwarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("organization"); got != "my-org" {
			t.Errorf("%s: organization = %q, want my-org", r.URL.Path, got)
		}
		if got := q.Get("product"); got != "copilot" {
			t.Errorf("%s: product = %q, want copilot", r.URL.Path, got)
		}
		if got := q.Get("cost_center_id"); got != "cc-1" {
			t.Errorf("%s: cost_center_id = %q, want cc-1", r.URL.Path, got)
		}
		fmt.Fprint(w, `{"enterprise":"test-ent","usageItems":[]}`)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	now := time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC)
	_, err := billing.GetPeriodComparison(c, "test-ent", billing.UsageFilter{
		Organization: "my-org",
		Product:      "copilot",
		CostCenterID: "cc-1",
	}, now)
	if err != nil {
		t.Fatalf("GetPeriodComparison error: %v", err)
	}
}

// TestGetPeriodComparison_errorWrapsPeriod verifies a failure on the
// previous-month fetch is wrapped with which period failed, so operators
// can tell at a glance which of the two periods had the problem. Uses 404
// (no retry/backoff in the client) to keep the test fast.
func TestGetPeriodComparison_errorWrapsPeriod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	now := time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC)
	_, err := billing.GetPeriodComparison(c, "test-ent", billing.UsageFilter{}, now)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "previous month (2026-06)") {
		t.Errorf("error = %q, want it to mention \"previous month (2026-06)\"", err.Error())
	}
}
