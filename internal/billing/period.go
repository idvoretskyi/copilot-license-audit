// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package billing

import (
	"fmt"
	"time"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// PeriodData holds every billing view (usage summary, premium requests, AI
// credits) for a single calendar month.
type PeriodData struct {
	Year            int                `json:"year"`
	Month           int                `json:"month"`
	Summary         *UsageSummary      `json:"summary"`
	PremiumRequests *ConsumptionReport `json:"premium_requests"`
	AICredits       *ConsumptionReport `json:"ai_credits"`
}

// PeriodComparison bundles two PeriodData snapshots so the previous (closed)
// calendar month and the current month to date can be reported side by side
// in a single command.
type PeriodComparison struct {
	Enterprise  string      `json:"enterprise"`
	GeneratedAt time.Time   `json:"generated_at"`
	Previous    *PeriodData `json:"previous_month"`
	Current     *PeriodData `json:"current_month_to_date"`
}

// PreviousPeriod returns the (year, month) of the calendar month immediately
// preceding the given (year, month), rolling over from January to December
// of the prior year.
func PreviousPeriod(year, month int) (int, int) {
	month--
	if month < 1 {
		month = 12
		year--
	}
	return year, month
}

// GetPeriodComparison fetches billing summary, premium-request, and
// AI-credit data for two periods in a single call: the previous (already
// closed) calendar month, and the current month to date. now is injected
// for testability; production callers pass time.Now().UTC().
//
// f.Year/f.Month/f.Day are ignored (each period sets its own); only
// f.Organization, f.Product, f.SKU, and f.CostCenterID are honored.
func GetPeriodComparison(c *client.Client, enterprise string, f UsageFilter, now time.Time) (*PeriodComparison, error) {
	curYear, curMonth := now.Year(), int(now.Month())
	prevYear, prevMonth := PreviousPeriod(curYear, curMonth)

	prev, err := getPeriodData(c, enterprise, f, prevYear, prevMonth)
	if err != nil {
		return nil, fmt.Errorf("previous month (%04d-%02d): %w", prevYear, prevMonth, err)
	}

	cur, err := getPeriodData(c, enterprise, f, curYear, curMonth)
	if err != nil {
		return nil, fmt.Errorf("current month (%04d-%02d): %w", curYear, curMonth, err)
	}

	return &PeriodComparison{
		Enterprise:  enterprise,
		GeneratedAt: now,
		Previous:    prev,
		Current:     cur,
	}, nil
}

// getPeriodData fetches all three billing views for a single (year, month).
func getPeriodData(c *client.Client, enterprise string, f UsageFilter, year, month int) (*PeriodData, error) {
	uf := f
	uf.Year, uf.Month, uf.Day = year, month, 0

	summary, err := GetUsageSummary(c, enterprise, uf)
	if err != nil {
		return nil, fmt.Errorf("usage summary: %w", err)
	}

	cf := ConsumptionFilter{
		Year:         year,
		Month:        month,
		Organization: f.Organization,
		Product:      f.Product,
		CostCenterID: f.CostCenterID,
	}

	premium, err := GetPremiumRequestUsage(c, enterprise, cf)
	if err != nil {
		return nil, fmt.Errorf("premium requests: %w", err)
	}

	credits, err := GetAICreditUsage(c, enterprise, cf)
	if err != nil {
		return nil, fmt.Errorf("ai credits: %w", err)
	}

	return &PeriodData{
		Year:            year,
		Month:           month,
		Summary:         summary,
		PremiumRequests: premium,
		AICredits:       credits,
	}, nil
}
