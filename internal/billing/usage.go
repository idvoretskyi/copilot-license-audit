// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// UsageItem is one line-item from the enhanced billing usage report.
// Endpoint: GET /enterprises/{enterprise}/settings/billing/usage
type UsageItem struct {
	Date             string  `json:"date"`
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	Quantity         float64 `json:"quantity"`
	UnitType         string  `json:"unitType"`
	PricePerUnit     float64 `json:"pricePerUnit"`
	GrossAmount      float64 `json:"grossAmount"`
	DiscountAmount   float64 `json:"discountAmount"`
	NetAmount        float64 `json:"netAmount"`
	OrganizationName string  `json:"organizationName"`
	RepositoryName   string  `json:"repositoryName,omitempty"`
}

// UsageReport is the response from the usage endpoint.
type UsageReport struct {
	UsageItems []UsageItem `json:"usageItems"`
}

// UsageSummaryItem is one aggregated line from the usage summary endpoint.
type UsageSummaryItem struct {
	Product          string  `json:"product"`
	SKU              string  `json:"sku"`
	UnitType         string  `json:"unitType"`
	PricePerUnit     float64 `json:"pricePerUnit"`
	GrossQuantity    float64 `json:"grossQuantity"`
	GrossAmount      float64 `json:"grossAmount"`
	DiscountQuantity float64 `json:"discountQuantity"`
	DiscountAmount   float64 `json:"discountAmount"`
	NetQuantity      float64 `json:"netQuantity"`
	NetAmount        float64 `json:"netAmount"`
}

// UsageSummary is the response from the usage/summary endpoint.
type UsageSummary struct {
	TimePeriod   TimePeriod         `json:"timePeriod"`
	Enterprise   string             `json:"enterprise"`
	Organization string             `json:"organization,omitempty"`
	Product      string             `json:"product,omitempty"`
	SKU          string             `json:"sku,omitempty"`
	UsageItems   []UsageSummaryItem `json:"usageItems"`
}

// UsageFilter holds optional query parameters for the usage endpoints.
type UsageFilter struct {
	Year         int
	Month        int
	Day          int
	Organization string
	Product      string
	SKU          string
	CostCenterID string
}

// values renders the non-zero fields as URL query parameters.
func (f UsageFilter) values() url.Values {
	q := url.Values{}
	if f.Year > 0 {
		q.Set("year", fmt.Sprintf("%d", f.Year))
	}
	if f.Month > 0 {
		q.Set("month", fmt.Sprintf("%d", f.Month))
	}
	if f.Day > 0 {
		q.Set("day", fmt.Sprintf("%d", f.Day))
	}
	if f.Organization != "" {
		q.Set("organization", f.Organization)
	}
	if f.Product != "" {
		q.Set("product", f.Product)
	}
	if f.SKU != "" {
		q.Set("sku", f.SKU)
	}
	if f.CostCenterID != "" {
		q.Set("cost_center_id", f.CostCenterID)
	}
	return q
}

// GetUsage retrieves the enhanced billing platform usage report.
// Endpoint: GET /enterprises/{enterprise}/settings/billing/usage
//
// NOTE: as of API version 2026-03-10, this endpoint accepts product/sku/
// organization query parameters but silently ignores them (verified against
// the live API: the same full, unfiltered item set is returned regardless
// of these parameters; only year/month/day are honored server-side). The
// summary endpoint (GetUsageSummary) does honor them correctly. To make
// this tool's --product/--sku/--org flags behave as documented regardless
// of that upstream inconsistency, results are also filtered client-side.
func GetUsage(c *client.Client, enterprise string, f UsageFilter) (*UsageReport, error) {
	report, err := client.Get[UsageReport](c, usageURL(c, enterprise, f))
	if err != nil {
		return nil, err
	}
	report.UsageItems = filterUsageItems(report.UsageItems, f)
	return report, nil
}

// filterUsageItems applies client-side product/sku/organization filtering,
// working around the upstream API's failure to honor these parameters on
// the /settings/billing/usage endpoint (see GetUsage doc comment).
func filterUsageItems(items []UsageItem, f UsageFilter) []UsageItem {
	if f.Product == "" && f.SKU == "" && f.Organization == "" {
		return items
	}
	out := make([]UsageItem, 0, len(items))
	for _, item := range items {
		if f.Product != "" && !strings.EqualFold(item.Product, f.Product) {
			continue
		}
		if f.SKU != "" && !strings.EqualFold(item.SKU, f.SKU) {
			continue
		}
		if f.Organization != "" && !strings.EqualFold(item.OrganizationName, f.Organization) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// GetUsageSummary retrieves the aggregated billing usage summary.
// Endpoint: GET /enterprises/{enterprise}/settings/billing/usage/summary
func GetUsageSummary(c *client.Client, enterprise string, f UsageFilter) (*UsageSummary, error) {
	return client.Get[UsageSummary](c, usageSummaryURL(c, enterprise, f))
}

// GetUsageJSON is a helper that returns JSON for --format json. It routes
// through GetUsage (rather than a raw passthrough) so the client-side
// product/sku/organization filter workaround applies consistently to both
// text and JSON output.
func GetUsageJSON(c *client.Client, enterprise string, f UsageFilter) (json.RawMessage, error) {
	report, err := GetUsage(c, enterprise, f)
	if err != nil {
		return nil, err
	}
	return json.Marshal(report)
}

// GetUsageSummaryJSON is a helper that returns the raw JSON for --format json.
func GetUsageSummaryJSON(c *client.Client, enterprise string, f UsageFilter) (json.RawMessage, error) {
	return client.GetRaw(c, usageSummaryURL(c, enterprise, f))
}

func usageURL(c *client.Client, enterprise string, f UsageFilter) string {
	return withQuery(c.URL(fmt.Sprintf("/enterprises/%s/settings/billing/usage", enterprise)), f.values())
}

func usageSummaryURL(c *client.Client, enterprise string, f UsageFilter) string {
	return withQuery(c.URL(fmt.Sprintf("/enterprises/%s/settings/billing/usage/summary", enterprise)), f.values())
}

// withQuery appends an already-populated url.Values to base, if any.
func withQuery(base string, q url.Values) string {
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}
