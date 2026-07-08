// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package copilot_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
	"github.com/idvoretskyi/copilot-license-audit/internal/copilot"
)

func makeSeat(login, org, plan string) map[string]any {
	return map[string]any{
		"assignee":     map[string]any{"login": login},
		"organization": map[string]any{"login": org},
		"plan_type":    plan,
		"created_at":   "2026-01-01T00:00:00Z",
	}
}

func TestListSeats_singlePage(t *testing.T) {
	seats := []map[string]any{
		makeSeat("alice", "org-a", "enterprise"),
		makeSeat("bob", "org-a", "enterprise"),
		makeSeat("carol", "org-b", "enterprise"),
	}
	body, _ := json.Marshal(map[string]any{"total_seats": 3, "seats": seats})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := client.New("tok", client.DefaultAPIVersion, srv.URL, false)
	got, total, err := copilot.ListSeats(c, "test-ent")
	if err != nil {
		t.Fatalf("ListSeats error: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(got) != 3 {
		t.Errorf("len(seats) = %d, want 3", len(got))
	}
	if got[0].Login != "alice" {
		t.Errorf("seats[0].Login = %q, want %q", got[0].Login, "alice")
	}
}

func TestListSeats_pagination(t *testing.T) {
	page1Seats := []map[string]any{makeSeat("alice", "org-a", "enterprise")}
	page2Seats := []map[string]any{makeSeat("bob", "org-b", "enterprise")}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			body, _ := json.Marshal(map[string]any{"total_seats": 2, "seats": page2Seats})
			w.Write(body)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/enterprises/test-ent/copilot/billing/seats?per_page=100&page=2>; rel="next"`, srv.URL))
		body, _ := json.Marshal(map[string]any{"total_seats": 2, "seats": page1Seats})
		w.Write(body)
	}))
	defer srv.Close()

	c := client.New("tok", client.DefaultAPIVersion, srv.URL, false)
	got, total, err := copilot.ListSeats(c, "test-ent")
	if err != nil {
		t.Fatalf("ListSeats error: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(got) != 2 {
		t.Errorf("len(seats) = %d, want 2", len(got))
	}
}

func TestListSeats_skipsEmptyLogins(t *testing.T) {
	seats := []map[string]any{
		{"assignee": map[string]any{"login": ""}, "organization": map[string]any{"login": "org"}, "plan_type": "enterprise", "created_at": "2026-01-01T00:00:00Z"},
		makeSeat("valid", "org", "enterprise"),
	}
	body, _ := json.Marshal(map[string]any{"total_seats": 2, "seats": seats})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	c := client.New("tok", client.DefaultAPIVersion, srv.URL, false)
	got, _, err := copilot.ListSeats(c, "test-ent")
	if err != nil {
		t.Fatalf("ListSeats error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(seats) = %d, want 1 (empty login skipped)", len(got))
	}
}

// TestAnalyze verifies classification, grouping, and Business detection.
func TestAnalyze(t *testing.T) {
	seats := []copilot.Seat{
		{Login: "alice", Org: "org-a", PlanType: "enterprise"},
		{Login: "bob", Org: "org-a", PlanType: "enterprise"},
		{Login: "carol", Org: "org-b", PlanType: "enterprise"},
		// Cross-org user
		{Login: "dave", Org: "org-a", PlanType: "enterprise"},
		{Login: "dave", Org: "org-b", PlanType: "enterprise"},
		// Stray Business seat
		{Login: "eve", Org: "org-c", PlanType: "business"},
		// Unknown plan
		{Login: "frank", Org: "org-d", PlanType: "unknown"},
	}

	result := copilot.Analyze(seats, map[string]bool{})

	if result.Classification["enterprise"] != 5 {
		t.Errorf("enterprise count = %d, want 5", result.Classification["enterprise"])
	}
	if result.Classification["business"] != 1 {
		t.Errorf("business count = %d, want 1", result.Classification["business"])
	}
	if result.Classification["unknown"] != 1 {
		t.Errorf("unknown count = %d, want 1", result.Classification["unknown"])
	}
	if len(result.BusinessSeats) != 1 || result.BusinessSeats[0].Login != "eve" {
		t.Errorf("BusinessSeats = %v, want [{eve org-c business}]", result.BusinessSeats)
	}
	if len(result.CrossOrgUsers) != 1 || result.CrossOrgUsers[0] != "dave" {
		t.Errorf("CrossOrgUsers = %v, want [dave]", result.CrossOrgUsers)
	}
	if result.DistinctEnterprise != 4 { // alice, bob, carol, dave (counted once)
		t.Errorf("DistinctEnterprise = %d, want 4", result.DistinctEnterprise)
	}
}

// TestAnalyze_duplicateRowSameOrgNotCrossOrg verifies that a login with two
// seat rows in the *same* org (e.g. assigned both directly and via a team,
// as observed against a live enterprise with a modest fraction of its
// users) is NOT counted as cross-org. Cross-org must require distinct
// orgs, not merely >1 row.
func TestAnalyze_duplicateRowSameOrgNotCrossOrg(t *testing.T) {
	seats := []copilot.Seat{
		{Login: "alice", Org: "org-a", PlanType: "enterprise"},
		{Login: "alice", Org: "org-a", PlanType: "enterprise"}, // duplicate row, same org
		{Login: "bob", Org: "org-a", PlanType: "enterprise"},
	}

	result := copilot.Analyze(seats, map[string]bool{})

	if len(result.CrossOrgUsers) != 0 {
		t.Errorf("CrossOrgUsers = %v, want empty (duplicate rows within same org)", result.CrossOrgUsers)
	}
	if result.DistinctEnterprise != 2 {
		t.Errorf("DistinctEnterprise = %d, want 2", result.DistinctEnterprise)
	}
	if len(result.ByOrg["org-a"]) != 2 {
		t.Errorf("ByOrg[org-a] = %v, want 2 distinct logins", result.ByOrg["org-a"])
	}
}

// TestAnalyze_excludeOrgs verifies org exclusion works.
func TestAnalyze_excludeOrgs(t *testing.T) {
	seats := []copilot.Seat{
		{Login: "alice", Org: "keep", PlanType: "enterprise"},
		{Login: "bob", Org: "skip", PlanType: "enterprise"},
	}
	result := copilot.Analyze(seats, map[string]bool{"skip": true})
	if _, ok := result.ByOrg["skip"]; ok {
		t.Error("excluded org 'skip' should not appear in ByOrg")
	}
	if _, ok := result.ByOrg["keep"]; !ok {
		t.Error("org 'keep' should appear in ByOrg")
	}
}

// TestSortedPlanKeys verifies enterprise comes before business.
func TestSortedPlanKeys(t *testing.T) {
	m := map[string]int{"business": 1, "enterprise": 5, "unknown": 2}
	keys := copilot.SortedPlanKeys(m)
	if keys[0] != "enterprise" {
		t.Errorf("keys[0] = %q, want enterprise", keys[0])
	}
	if keys[1] != "business" {
		t.Errorf("keys[1] = %q, want business", keys[1])
	}
}

// TestSortedOrgs verifies descending-count ordering.
func TestSortedOrgs(t *testing.T) {
	byOrg := map[string][]string{
		"small": {"a"},
		"large": {"a", "b", "c"},
		"mid":   {"a", "b"},
	}
	orgs := copilot.SortedOrgs(byOrg)
	if orgs[0] != "large" {
		t.Errorf("orgs[0] = %q, want large", orgs[0])
	}
	if orgs[1] != "mid" {
		t.Errorf("orgs[1] = %q, want mid", orgs[1])
	}
	if orgs[2] != "small" {
		t.Errorf("orgs[2] = %q, want small", orgs[2])
	}
}
