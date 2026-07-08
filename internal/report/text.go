// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package report provides human-readable (text) and machine-readable (JSON)
// output renderers for every subcommand.  All renderers write to an io.Writer
// so tests can capture output without os.Stdout.
package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/idvoretskyi/copilot-license-audit/internal/billing"
	"github.com/idvoretskyi/copilot-license-audit/internal/copilot"
	"github.com/idvoretskyi/copilot-license-audit/internal/metrics"
)

const rule = "─────────────────────────────────────────────────────────────"

// ---- Audit (seat inventory + health check) --------------------------------

// AuditText prints the full seat classification, Enterprise inventory, and
// (if any stray Business seats exist) a regression remediation block.
func AuditText(w io.Writer, enterprise string, totalSeats int, result copilot.AuditResult, excludeOrgs map[string]bool) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Copilot Enterprise Seat Inventory — enterprise: %s\n", enterprise)
	fmt.Fprintln(w, rule)
	fmt.Fprintf(w, "  Total seats reported by API : %d\n\n", totalSeats)

	fmt.Fprintln(w, "  Seat classification (all plan_type values):")
	for _, k := range copilot.SortedPlanKeys(result.Classification) {
		count := result.Classification[k]
		var tag string
		switch k {
		case copilot.PlanEnterprise:
			tag = "← active (migration complete)"
		case copilot.PlanBusiness:
			tag = "← REGRESSION: Business seats should be 0"
		case copilot.PlanUnknown:
			tag = "← unknown plan_type (see GitHub API docs)"
		default:
			tag = "← unrecognised / transitional"
		}
		fmt.Fprintf(w, "    %-12s : %4d  %s\n", k, count, tag)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Enterprise seat rows      : %d\n", result.TotalEnterprise)
	fmt.Fprintf(w, "  Distinct Enterprise users : %d\n", result.DistinctEnterprise)

	if len(result.CrossOrgUsers) > 0 {
		fmt.Fprintf(w, "  Cross-org users           : %d (assigned in multiple orgs)\n",
			len(result.CrossOrgUsers))
		for _, login := range result.CrossOrgUsers {
			var orgs []string
			for o, logins := range result.ByOrg {
				for _, l := range logins {
					if l == login {
						orgs = append(orgs, o)
					}
				}
			}
			fmt.Fprintf(w, "    %s  →  %s\n", login, strings.Join(orgs, ", "))
		}
	}

	fmt.Fprintf(w, "  Organisations             : %d\n", len(result.ByOrg))
	if len(excludeOrgs) > 0 {
		excluded := make([]string, 0, len(excludeOrgs))
		for o := range excludeOrgs {
			excluded = append(excluded, o)
		}
		fmt.Fprintf(w, "  Excluded orgs             : %s\n", strings.Join(excluded, ", "))
	}

	if result.TotalEnterprise > 0 {
		fmt.Fprintf(w, "\n  Per-org breakdown:\n\n")
		for _, org := range copilot.SortedOrgs(result.ByOrg) {
			logins := result.ByOrg[org]
			fmt.Fprintf(w, "    [%s] — %d seat(s)\n", org, len(logins))
			for i, login := range logins {
				fmt.Fprintf(w, "      %3d. %s\n", i+1, login)
			}
			fmt.Fprintln(w)
		}
	}

	// Health-check result.
	fmt.Fprintln(w, rule)
	if len(result.BusinessSeats) == 0 {
		fmt.Fprintln(w, "  Health check: PASS — no Business seats detected.")
		fmt.Fprintln(w, "  Migration to Copilot Enterprise is complete.")
	} else {
		fmt.Fprintln(w, "  Health check: FAIL — stray Business seats detected!")
		fmt.Fprintf(w, "  %d Business seat(s) found across %d org(s):\n\n",
			len(result.BusinessSeats), countBusinessOrgs(result.BusinessSeats))
		printBusinessSeats(w, result.BusinessSeats)
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Remediation: each affected org's Owner must cancel the")
		fmt.Fprintln(w, "  Copilot Business subscription via the GitHub UI:")
		fmt.Fprintln(w, "    Org Settings → Copilot → Manage subscription → Cancel")
	}
	fmt.Fprintln(w)
}

func countBusinessOrgs(seats []copilot.Seat) int {
	orgs := map[string]struct{}{}
	for _, s := range seats {
		orgs[s.Org] = struct{}{}
	}
	return len(orgs)
}

func printBusinessSeats(w io.Writer, seats []copilot.Seat) {
	currentOrg := ""
	i := 1
	for _, s := range seats {
		if s.Org != currentOrg {
			currentOrg = s.Org
			fmt.Fprintf(w, "    [%s]\n", s.Org)
			i = 1
		}
		fmt.Fprintf(w, "      %3d. %s\n", i, s.Login)
		i++
	}
}

// ---- Billing: usage -------------------------------------------------------

// BillingUsageText renders the raw enhanced-billing usage line items: one
// row per product/SKU/date/organization.
func BillingUsageText(w io.Writer, report *billing.UsageReport) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Billing Usage Report")
	fmt.Fprintln(w, rule)
	if len(report.UsageItems) == 0 {
		fmt.Fprintln(w, "  No usage items found for the requested period.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "  %-12s  %-30s  %-20s  %10s  %10s  %10s\n",
		"Date", "Product", "SKU", "Qty", "GrossAmt", "NetAmt")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 100))
	for _, item := range report.UsageItems {
		fmt.Fprintf(w, "  %-12s  %-30s  %-20s  %10.2f  %10.4f  %10.4f\n",
			item.Date, truncate(item.Product, 30), truncate(item.SKU, 20),
			item.Quantity, item.GrossAmount, item.NetAmount)
	}
	fmt.Fprintln(w)
}

// BillingUsageSummaryText renders the aggregated billing usage summary for a
// single period: enterprise, period, optional product filter, and the item
// table with its net total.
func BillingUsageSummaryText(w io.Writer, summary *billing.UsageSummary) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Billing Usage Summary")
	fmt.Fprintln(w, rule)
	fmt.Fprintf(w, "  Enterprise  : %s\n", summary.Enterprise)
	fmt.Fprintf(w, "  Period      : %s\n", formatPeriod(summary.TimePeriod))
	if summary.Product != "" {
		fmt.Fprintf(w, "  Product     : %s\n", summary.Product)
	}
	fmt.Fprintln(w)
	billingUsageSummaryTable(w, summary.UsageItems)
	fmt.Fprintln(w)
}

// billingUsageSummaryTable renders just the item table (no title/enterprise/
// period header) and returns the total net amount, so it can be reused both
// by BillingUsageSummaryText and by the multi-period BillingPeriodReportText.
func billingUsageSummaryTable(w io.Writer, items []billing.UsageSummaryItem) float64 {
	if len(items) == 0 {
		fmt.Fprintln(w, "  No usage items found.")
		return 0
	}
	fmt.Fprintf(w, "  %-30s  %-20s  %12s  %12s  %12s\n",
		"Product", "SKU", "GrossQty", "Discount", "NetAmt")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 92))
	var totalNet float64
	for _, item := range items {
		fmt.Fprintf(w, "  %-30s  %-20s  %12.2f  %12.4f  %12.4f\n",
			truncate(item.Product, 30), truncate(item.SKU, 20),
			item.GrossQuantity, item.DiscountAmount, item.NetAmount)
		totalNet += item.NetAmount
	}
	fmt.Fprintln(w, "  "+strings.Repeat("-", 92))
	fmt.Fprintf(w, "  %-54s  %12s  %12.4f\n", "TOTAL", "", totalNet)
	return totalNet
}

// ---- Billing: consumption (premium requests / AI credits) -----------------

// ConsumptionText renders a premium-request or AI-credit consumption report
// (kind distinguishes the two in the title, e.g. "Premium Request").
func ConsumptionText(w io.Writer, kind string, report *billing.ConsumptionReport) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s Consumption Report\n", kind)
	fmt.Fprintln(w, rule)
	fmt.Fprintf(w, "  Enterprise : %s\n", report.Enterprise)
	fmt.Fprintf(w, "  Period     : %s\n", formatPeriod(report.TimePeriod))
	fmt.Fprintln(w)
	consumptionTable(w, report.UsageItems)
	fmt.Fprintln(w)
}

// consumptionTable renders just the item table (no title/enterprise/period
// header) and returns the total net amount, so it can be reused both by
// ConsumptionText and by the multi-period BillingPeriodReportText.
func consumptionTable(w io.Writer, items []billing.ConsumptionItem) float64 {
	if len(items) == 0 {
		fmt.Fprintln(w, "  No usage items found for the requested period.")
		return 0
	}
	fmt.Fprintf(w, "  %-20s  %-20s  %-20s  %12s  %12s\n",
		"Product", "SKU", "Model", "NetQty", "NetAmt")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 90))
	var totalNet float64
	for _, item := range items {
		fmt.Fprintf(w, "  %-20s  %-20s  %-20s  %12.2f  %12.4f\n",
			truncate(item.Product, 20), truncate(item.SKU, 20),
			truncate(item.Model, 20), item.NetQuantity, item.NetAmount)
		totalNet += item.NetAmount
	}
	fmt.Fprintln(w, "  "+strings.Repeat("-", 90))
	fmt.Fprintf(w, "  %-64s  %12.4f\n", "TOTAL", totalNet)
	return totalNet
}

// ---- Billing: two-period report --------------------------------------------

// BillingPeriodReportText renders the previous (closed) calendar month
// alongside the current month to date, each broken down into Billing
// Summary, Premium Requests, and AI Credits, followed by a comparison
// footer. The comparison is based on the Billing Summary net totals only:
// premium requests and AI credits are detailed breakdowns already reflected
// in the Copilot line(s) of that summary, not additional charges, so they
// are deliberately not added on top to avoid double-counting.
func BillingPeriodReportText(w io.Writer, pc *billing.PeriodComparison) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Two-Period Billing Report — enterprise: %s\n", pc.Enterprise)
	fmt.Fprintln(w, rule)
	fmt.Fprintf(w, "  Generated : %s\n", pc.GeneratedAt.Format(time.RFC3339))

	prevTotal := billingPeriodSection(w, "Previous month (closed)", pc.Previous, "")

	elapsed := pc.GeneratedAt.Day()
	total := daysInMonth(pc.Current.Year, pc.Current.Month)
	curSuffix := fmt.Sprintf("day %d of %d, %.1f%% elapsed", elapsed, total, 100*float64(elapsed)/float64(total))
	curTotal := billingPeriodSection(w, "Current month to date", pc.Current, curSuffix)

	fmt.Fprintln(w, rule)
	fmt.Fprintln(w, "  Comparison (Billing Summary net totals — premium requests and AI")
	fmt.Fprintln(w, "  credits above are breakdowns already reflected in these totals,")
	fmt.Fprintln(w, "  not additional charges):")
	fmt.Fprintf(w, "    Previous month (closed, %04d-%02d) : %12.4f\n",
		pc.Previous.Year, pc.Previous.Month, prevTotal)
	fmt.Fprintf(w, "    Current month to date (%04d-%02d)  : %12.4f  (%s)\n",
		pc.Current.Year, pc.Current.Month, curTotal, curSuffix)
	if prevTotal != 0 {
		fmt.Fprintf(w, "    MTD is %.1f%% of the previous closed month's total\n", 100*curTotal/prevTotal)
	}
	fmt.Fprintln(w)
}

// billingPeriodSection renders one period's title + all three tables and
// returns the Billing Summary net total for that period.
func billingPeriodSection(w io.Writer, title string, pd *billing.PeriodData, suffix string) float64 {
	fmt.Fprintln(w)
	if suffix != "" {
		fmt.Fprintf(w, "%s: %04d-%02d (%s)\n", title, pd.Year, pd.Month, suffix)
	} else {
		fmt.Fprintf(w, "%s: %04d-%02d\n", title, pd.Year, pd.Month)
	}
	fmt.Fprintln(w, rule)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Billing Summary:")
	total := billingUsageSummaryTable(w, pd.Summary.UsageItems)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Premium Requests:")
	consumptionTable(w, pd.PremiumRequests.UsageItems)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "  AI Credits:")
	consumptionTable(w, pd.AICredits.UsageItems)

	return total
}

// daysInMonth returns the number of days in the given calendar month,
// correctly handling leap years via time.Date's day-0-of-next-month
// normalization.
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ---- Billing: budgets -----------------------------------------------------

// BudgetsText renders the enterprise's configured spending budgets.
func BudgetsText(w io.Writer, budgets []billing.Budget) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Enterprise Budgets (read-only)")
	fmt.Fprintln(w, rule)
	if len(budgets) == 0 {
		fmt.Fprintln(w, "  No budgets configured.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "  %-38s  %-14s  %-20s  %8s  %s\n",
		"ID", "Scope", "Product SKU", "Amount", "Block?")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 96))
	for _, b := range budgets {
		entity := b.BudgetEntityName
		if entity == "" {
			entity = b.User
		}
		scope := b.BudgetScope
		if entity != "" {
			scope += "/" + entity
		}
		fmt.Fprintf(w, "  %-38s  %-14s  %-20s  %8d  %v\n",
			truncate(b.ID, 38), truncate(scope, 14),
			truncate(b.BudgetProductSKU, 20), b.BudgetAmount, b.PreventUsage)
	}
	fmt.Fprintln(w)
}

// ---- Metrics reports -------------------------------------------------------

// MetricsReportText renders the signed download links for a Copilot
// Enterprise usage metrics report at the given scope.
func MetricsReportText(w io.Writer, scope metrics.ReportScope, links *metrics.ReportLinks) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Copilot Enterprise Metrics Report — scope: %s\n", scope)
	fmt.Fprintln(w, rule)
	if links.ReportDay != "" {
		fmt.Fprintf(w, "  Report day  : %s\n", links.ReportDay)
	}
	if links.ReportStartDay != "" {
		fmt.Fprintf(w, "  Report period: %s → %s\n", links.ReportStartDay, links.ReportEndDay)
	}
	fmt.Fprintf(w, "  Download links (%d):\n", len(links.DownloadLinks))
	for i, dl := range links.DownloadLinks {
		// Shorten the signed URL at the '?' to keep output readable.
		display := dl
		if stripped := metrics.StripQuery(dl); stripped != dl {
			display = stripped + "  [+ signed query params]"
		}
		fmt.Fprintf(w, "    %d. %s\n", i+1, display)
	}
	fmt.Fprintln(w)
}

// MetricsDownloadedText lists the local file paths written by --download.
func MetricsDownloadedText(w io.Writer, paths []string) {
	fmt.Fprintln(w, "  Downloaded files:")
	for _, p := range paths {
		fmt.Fprintf(w, "    %s\n", p)
	}
	fmt.Fprintln(w)
}

// ---- Helpers ---------------------------------------------------------------

// formatPeriod renders a billing.TimePeriod at its natural granularity:
// YYYY-MM-DD if Day is set, YYYY-MM if Month is set, else just YYYY.
func formatPeriod(tp billing.TimePeriod) string {
	if tp.Day > 0 {
		return fmt.Sprintf("%04d-%02d-%02d", tp.Year, tp.Month, tp.Day)
	}
	if tp.Month > 0 {
		return fmt.Sprintf("%04d-%02d", tp.Year, tp.Month)
	}
	return fmt.Sprintf("%04d", tp.Year)
}

// truncate shortens s to at most n runes, replacing the last one with "…"
// when it doesn't fit. Operates on runes (not bytes) so multi-byte UTF-8
// names (e.g. non-ASCII org or SKU names) are never cut mid-rune.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
