// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idvoretskyi/copilot-license-audit/internal/billing"
	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

func newClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	return client.New("tok", client.DefaultAPIVersion, baseURL, false)
}

// TestGetUsage verifies basic usage report parsing.
func TestGetUsage(t *testing.T) {
	resp := billing.UsageReport{
		UsageItems: []billing.UsageItem{
			{Date: "2026-06-01", Product: "copilot", SKU: "copilot_enterprise_seat", Quantity: 50, NetAmount: 1950.00},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enterprises/test-ent/settings/billing/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.GetUsage(c, "test-ent", billing.UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsage error: %v", err)
	}
	if len(got.UsageItems) != 1 {
		t.Errorf("UsageItems count = %d, want 1", len(got.UsageItems))
	}
	if got.UsageItems[0].Product != "copilot" {
		t.Errorf("Product = %q, want copilot", got.UsageItems[0].Product)
	}
}

// TestGetUsage_decimalQuantity verifies large, non-integer quantity values
// (as actually returned by the live enhanced billing API, e.g. 25098682.0
// for actions_storage) unmarshal without error.
func TestGetUsage_decimalQuantity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"usageItems":[{"date":"2026-07-01","product":"actions","sku":"actions_storage","quantity":25098682.0,"netAmount":19.5856}]}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.GetUsage(c, "test-ent", billing.UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsage error: %v", err)
	}
	if len(got.UsageItems) != 1 {
		t.Fatalf("UsageItems count = %d, want 1", len(got.UsageItems))
	}
	if got.UsageItems[0].Quantity != 25098682.0 {
		t.Errorf("Quantity = %v, want 25098682.0", got.UsageItems[0].Quantity)
	}
}

// TestGetUsage_withFilter verifies query parameters are forwarded.
func TestGetUsage_withFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("year") != "2026" {
			t.Errorf("year = %q, want 2026", q.Get("year"))
		}
		if q.Get("month") != "5" {
			t.Errorf("month = %q, want 5", q.Get("month"))
		}
		if q.Get("product") != "copilot" {
			t.Errorf("product = %q, want copilot", q.Get("product"))
		}
		w.Write([]byte(`{"usageItems":[]}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	_, err := billing.GetUsage(c, "test-ent", billing.UsageFilter{Year: 2026, Month: 5, Product: "copilot"})
	if err != nil {
		t.Fatalf("GetUsage error: %v", err)
	}
}

// TestGetUsage_clientSideProductFilter verifies that GetUsage filters
// results by product client-side, working around the live enhanced billing
// API's confirmed failure to honor the "product" query parameter on the
// /settings/billing/usage endpoint (it returns every product regardless of
// the filter; only the /usage/summary endpoint honors it server-side).
func TestGetUsage_clientSideProductFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate the real API: ignore the product filter, return everything.
		w.Write([]byte(`{"usageItems":[
			{"product":"actions","sku":"actions_linux","quantity":10,"netAmount":1.0},
			{"product":"copilot","sku":"copilot_enterprise","quantity":5,"netAmount":2.0},
			{"product":"Copilot","sku":"copilot_dotcom_chat","quantity":1,"netAmount":0.5}
		]}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.GetUsage(c, "test-ent", billing.UsageFilter{Product: "copilot"})
	if err != nil {
		t.Fatalf("GetUsage error: %v", err)
	}
	if len(got.UsageItems) != 2 {
		t.Fatalf("UsageItems count = %d, want 2 (case-insensitive product match)", len(got.UsageItems))
	}
	for _, item := range got.UsageItems {
		if !strings.EqualFold(item.Product, "copilot") {
			t.Errorf("unexpected product %q leaked through filter", item.Product)
		}
	}
}

// TestGetUsageJSON_appliesFilter verifies the --format json path is filtered
// consistently with the text path (both route through GetUsage).
func TestGetUsageJSON_appliesFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"usageItems":[
			{"product":"actions","sku":"actions_linux","quantity":10,"netAmount":1.0},
			{"product":"copilot","sku":"copilot_enterprise","quantity":5,"netAmount":2.0}
		]}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	raw, err := billing.GetUsageJSON(c, "test-ent", billing.UsageFilter{Product: "copilot"})
	if err != nil {
		t.Fatalf("GetUsageJSON error: %v", err)
	}
	var got billing.UsageReport
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got.UsageItems) != 1 || got.UsageItems[0].Product != "copilot" {
		t.Errorf("UsageItems = %+v, want 1 copilot item", got.UsageItems)
	}
}

// TestGetUsageSummary verifies summary parsing and total calculation.
func TestGetUsageSummary(t *testing.T) {
	resp := map[string]any{
		"timePeriod": map[string]any{"year": 2026, "month": 6},
		"enterprise": "test-ent",
		"usageItems": []map[string]any{
			{"product": "copilot", "sku": "seat", "netAmount": 500.0, "netQuantity": 10.0},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.GetUsageSummary(c, "test-ent", billing.UsageFilter{})
	if err != nil {
		t.Fatalf("GetUsageSummary error: %v", err)
	}
	if got.Enterprise != "test-ent" {
		t.Errorf("Enterprise = %q, want test-ent", got.Enterprise)
	}
	if len(got.UsageItems) != 1 {
		t.Fatalf("UsageItems count = %d, want 1", len(got.UsageItems))
	}
	if got.UsageItems[0].NetAmount != 500.0 {
		t.Errorf("NetAmount = %f, want 500", got.UsageItems[0].NetAmount)
	}
}

// TestGetPremiumRequestUsage verifies premium request endpoint is called correctly.
func TestGetPremiumRequestUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enterprises/test-ent/settings/billing/premium_request/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"enterprise":"test-ent","usageItems":[],"timePeriod":{"year":2026}}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.GetPremiumRequestUsage(c, "test-ent", billing.ConsumptionFilter{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.Enterprise != "test-ent" {
		t.Errorf("Enterprise = %q, want test-ent", got.Enterprise)
	}
}

// TestGetAICreditUsage verifies AI credit endpoint path.
func TestGetAICreditUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enterprises/test-ent/settings/billing/ai_credit/usage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"enterprise":"test-ent","usageItems":[],"timePeriod":{"year":2026}}`))
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	_, err := billing.GetAICreditUsage(c, "test-ent", billing.ConsumptionFilter{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
}

// TestListBudgets verifies budget list pagination using has_next_page.
func TestListBudgets(t *testing.T) {
	page1 := map[string]any{
		"budgets": []map[string]any{
			{"id": "b1", "budget_type": "ProductPricing", "budget_amount": 100,
				"prevent_further_usage": true, "budget_scope": "enterprise",
				"budget_product_sku": "copilot",
				"budget_alerting":    map[string]any{"will_alert": false, "alert_recipients": []string{}}},
		},
		"has_next_page": true,
		"total_count":   2,
	}
	page2 := map[string]any{
		"budgets": []map[string]any{
			{"id": "b2", "budget_type": "SkuPricing", "budget_amount": 50,
				"prevent_further_usage": false, "budget_scope": "organization",
				"budget_product_sku": "copilot_enterprise_seat",
				"budget_alerting":    map[string]any{"will_alert": true, "alert_recipients": []string{"admin"}}},
		},
		"has_next_page": false,
		"total_count":   2,
	}
	bodies := [][]byte{}
	b1, _ := json.Marshal(page1)
	b2, _ := json.Marshal(page2)
	bodies = append(bodies, b1, b2)

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(bodies[call])
		call++
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := billing.ListBudgets(c, "test-ent", billing.BudgetFilter{})
	if err != nil {
		t.Fatalf("ListBudgets error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(budgets) = %d, want 2", len(got))
	}
	if got[0].ID != "b1" {
		t.Errorf("budgets[0].ID = %q, want b1", got[0].ID)
	}
	if got[1].ID != "b2" {
		t.Errorf("budgets[1].ID = %q, want b2", got[1].ID)
	}
}

// TestListBudgets_scopeFilter verifies the scope query param is sent.
func TestListBudgets_scopeFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("scope"); got != "enterprise" {
			t.Errorf("scope = %q, want enterprise", got)
		}
		fmt.Fprint(w, `{"budgets":[],"has_next_page":false,"total_count":0}`)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	_, err := billing.ListBudgets(c, "test-ent", billing.BudgetFilter{Scope: "enterprise"})
	if err != nil {
		t.Fatalf("ListBudgets error: %v", err)
	}
}
