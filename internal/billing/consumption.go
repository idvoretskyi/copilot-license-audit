// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// ConsumptionItem is shared between premium-request and AI-credit reports.
type ConsumptionItem struct {
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	Model            string  `json:"model"`
	UnitType         string  `json:"unitType"`
	PricePerUnit     float64 `json:"pricePerUnit"`
	GrossQuantity    float64 `json:"grossQuantity"`
	GrossAmount      float64 `json:"grossAmount"`
	DiscountQuantity float64 `json:"discountQuantity"`
	DiscountAmount   float64 `json:"discountAmount"`
	NetQuantity      float64 `json:"netQuantity"`
	NetAmount        float64 `json:"netAmount"`
}

// ConsumptionReport is the response from the premium_request and ai_credit endpoints.
type ConsumptionReport struct {
	TimePeriod   TimePeriod        `json:"timePeriod"`
	Enterprise   string            `json:"enterprise"`
	User         string            `json:"user,omitempty"`
	Organization string            `json:"organization,omitempty"`
	Product      string            `json:"product,omitempty"`
	Model        string            `json:"model,omitempty"`
	UsageItems   []ConsumptionItem `json:"usageItems"`
}

// ConsumptionFilter holds optional query parameters for premium/AI-credit endpoints.
type ConsumptionFilter struct {
	Year         int
	Month        int
	Day          int
	Organization string
	User         string
	Model        string
	Product      string
	CostCenterID string
}

// values renders the non-zero fields as URL query parameters, properly escaped.
func (f ConsumptionFilter) values() url.Values {
	q := UsageFilter{
		Year:         f.Year,
		Month:        f.Month,
		Day:          f.Day,
		Organization: f.Organization,
		Product:      f.Product,
		CostCenterID: f.CostCenterID,
	}.values()
	if f.User != "" {
		q.Set("user", f.User)
	}
	if f.Model != "" {
		q.Set("model", f.Model)
	}
	return q
}

// GetPremiumRequestUsage retrieves the premium-request consumption report.
// Endpoint: GET /enterprises/{enterprise}/settings/billing/premium_request/usage
func GetPremiumRequestUsage(c *client.Client, enterprise string, f ConsumptionFilter) (*ConsumptionReport, error) {
	return client.Get[ConsumptionReport](c, premiumRequestURL(c, enterprise, f))
}

// GetAICreditUsage retrieves the AI-credit consumption report.
// Endpoint: GET /enterprises/{enterprise}/settings/billing/ai_credit/usage
func GetAICreditUsage(c *client.Client, enterprise string, f ConsumptionFilter) (*ConsumptionReport, error) {
	return client.Get[ConsumptionReport](c, aiCreditURL(c, enterprise, f))
}

// GetPremiumRequestUsageJSON returns raw JSON for --format json.
func GetPremiumRequestUsageJSON(c *client.Client, enterprise string, f ConsumptionFilter) (json.RawMessage, error) {
	return client.GetRaw(c, premiumRequestURL(c, enterprise, f))
}

// GetAICreditUsageJSON returns raw JSON for --format json.
func GetAICreditUsageJSON(c *client.Client, enterprise string, f ConsumptionFilter) (json.RawMessage, error) {
	return client.GetRaw(c, aiCreditURL(c, enterprise, f))
}

func premiumRequestURL(c *client.Client, enterprise string, f ConsumptionFilter) string {
	base := c.URL(fmt.Sprintf("/enterprises/%s/settings/billing/premium_request/usage", enterprise))
	return withQuery(base, f.values())
}

func aiCreditURL(c *client.Client, enterprise string, f ConsumptionFilter) string {
	base := c.URL(fmt.Sprintf("/enterprises/%s/settings/billing/ai_credit/usage", enterprise))
	return withQuery(base, f.values())
}
