// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package billing provides access to the GitHub Enhanced Billing Platform APIs:
// usage line items, aggregated summaries, premium-request and AI-credit
// consumption, enterprise budgets, and a two-period (previous month +
// month-to-date) convenience report. All endpoints are read-only (GET).
package billing

// TimePeriod is the billing period a report covers, shared by every
// enhanced-billing-platform response (usage summary, premium requests, AI
// credits). Day and Month are omitted from JSON when the report is at
// month or year granularity, respectively.
type TimePeriod struct {
	Year  int `json:"year"`
	Month int `json:"month,omitempty"`
	Day   int `json:"day,omitempty"`
}
