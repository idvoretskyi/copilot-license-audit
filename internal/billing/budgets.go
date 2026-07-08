// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// Budget represents a single enterprise budget entry.
type Budget struct {
	ID               string   `json:"id"`
	BudgetType       string   `json:"budget_type"`
	BudgetAmount     int      `json:"budget_amount"`
	PreventUsage     bool     `json:"prevent_further_usage"`
	BudgetScope      string   `json:"budget_scope"`
	BudgetEntityName string   `json:"budget_entity_name,omitempty"`
	User             string   `json:"user,omitempty"`
	BudgetProductSKU string   `json:"budget_product_sku"`
	BudgetAlerting   Alerting `json:"budget_alerting"`
}

// Alerting holds the alert configuration for a budget.
type Alerting struct {
	WillAlert       bool     `json:"will_alert"`
	AlertRecipients []string `json:"alert_recipients"`
}

// BudgetFilter holds optional query parameters for the budgets list endpoint.
type BudgetFilter struct {
	Scope string // enterprise | organization | repository | cost_center | user
	User  string
}

// values renders the filter as query parameters, including the fixed page size.
func (f BudgetFilter) values() url.Values {
	q := url.Values{}
	q.Set("per_page", "100")
	if f.Scope != "" {
		q.Set("scope", f.Scope)
	}
	if f.User != "" {
		q.Set("user", f.User)
	}
	return q
}

// ListBudgets returns all budgets for the enterprise (paginated via has_next_page).
// Endpoint: GET /enterprises/{enterprise}/settings/billing/budgets
// Read-only — no create / update / delete operations are implemented.
func ListBudgets(c *client.Client, enterprise string, f BudgetFilter) ([]Budget, error) {
	return fetchBudgetPages[Budget](c, budgetsURL(c, enterprise, f))
}

// ListBudgetsJSON returns the raw JSON of every page, flattened into a single
// array, for --format json.
func ListBudgetsJSON(c *client.Client, enterprise string, f BudgetFilter) (json.RawMessage, error) {
	items, err := fetchBudgetPages[json.RawMessage](c, budgetsURL(c, enterprise, f))
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(items)
	return json.RawMessage(out), err
}

func budgetsURL(c *client.Client, enterprise string, f BudgetFilter) string {
	base := c.URL(fmt.Sprintf("/enterprises/%s/settings/billing/budgets", enterprise))
	return withQuery(base, f.values())
}

// budgetsPage is the generic shape shared by ListBudgets (typed items) and
// ListBudgetsJSON (raw items) — only the "budgets" element type differs.
type budgetsPage[T any] struct {
	Budgets     []T  `json:"budgets"`
	HasNextPage bool `json:"has_next_page"`
}

// fetchBudgetPages walks every page of baseURL (which must already carry its
// query string, incl. "?") accumulating the "budgets" array, following
// has_next_page.
func fetchBudgetPages[T any](c *client.Client, baseURL string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var bp budgetsPage[T]
		if err := c.GetJSON(fmt.Sprintf("%s&page=%d", baseURL, page), &bp); err != nil {
			return nil, err
		}
		all = append(all, bp.Budgets...)
		if !bp.HasNextPage {
			return all, nil
		}
	}
}
