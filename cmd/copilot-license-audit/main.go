// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// copilot-license-audit — Copilot Enterprise seat auditing and billing reporting.
//
// Run with -h, or see README.md, for the full subcommand and flag reference.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/idvoretskyi/copilot-license-audit/internal/auth"
	"github.com/idvoretskyi/copilot-license-audit/internal/billing"
	"github.com/idvoretskyi/copilot-license-audit/internal/client"
	"github.com/idvoretskyi/copilot-license-audit/internal/copilot"
	"github.com/idvoretskyi/copilot-license-audit/internal/metrics"
	"github.com/idvoretskyi/copilot-license-audit/internal/report"
)

// version is set at build time via -ldflags "-X main.version=…"
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the real entry point, split out for testability.
func run(args []string) int {
	// Top-level flags (before the subcommand).
	fs := flag.NewFlagSet("copilot-license-audit", flag.ContinueOnError)
	enterprise := fs.String("enterprise", "", "GitHub Enterprise slug (required)")
	apiVersion := fs.String("api-version", client.DefaultAPIVersion, "X-GitHub-Api-Version header")
	apiURL := fs.String("api-url", "", "GitHub API base URL (default: https://api.github.com, or $GITHUB_API_URL; set for GHE Data Residency, e.g. https://api.example.ghe.com)")
	format := fs.String("format", "text", "Output format: text | json")
	verbose := fs.Bool("verbose", false, "Enable debug output")
	showVersion := fs.Bool("version", false, "Print version and exit")

	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(args); err != nil {
		if isHelpErr(err) {
			return client.ExitOK
		}
		return client.ExitError
	}

	if *showVersion {
		fmt.Println("copilot-license-audit", version)
		return client.ExitOK
	}

	// Determine subcommand from remaining args.
	sub := fs.Args()
	cmd, cmdArgs := parseSubcommand(sub)

	if cmd == "help" || cmd == "--help" {
		printUsage(fs)
		return client.ExitOK
	}

	// --enterprise is required for every real subcommand.
	if *enterprise == "" {
		fmt.Fprintln(os.Stderr, "Error: --enterprise is required")
		fmt.Fprintln(os.Stderr)
		printUsage(fs)
		return client.ExitError
	}

	if *format != "text" && *format != "json" {
		fmt.Fprintf(os.Stderr, "Error: --format must be 'text' or 'json', got %q\n", *format)
		return client.ExitError
	}

	// Authenticate.
	if !*verbose {
		fmt.Fprint(os.Stderr, "Authenticating... ")
	}
	token, err := auth.Token(*verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		return client.ExitError
	}
	if !*verbose {
		fmt.Fprintln(os.Stderr, "OK")
	}

	baseURL := *apiURL
	if baseURL == "" {
		baseURL = os.Getenv("GITHUB_API_URL")
	}
	c := client.New(token, *apiVersion, baseURL, *verbose)

	switch cmd {
	case "", "audit":
		return cmdAudit(c, *enterprise, *format, cmdArgs)
	case "billing":
		return cmdBilling(c, *enterprise, *format, cmdArgs)
	case "metrics":
		return cmdMetrics(c, *enterprise, *format, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown subcommand %q\n\n", cmd)
		printUsage(fs)
		return client.ExitError
	}
}

// ---- audit ----------------------------------------------------------------

func cmdAudit(c *client.Client, enterprise, format string, args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	excludeOrgsFlag := fs.String("exclude-orgs", "", "Comma-separated org names to exclude from the inventory")
	strict := fs.Bool("strict", false, "Exit 2 if any Business seats are detected (post-migration guard)")
	expectCount := fs.Int("expect-count", 0, "Expected distinct Enterprise users; exit 2 if actual differs (0=disabled)")
	if err := fs.Parse(args); err != nil {
		if isHelpErr(err) {
			return client.ExitOK
		}
		return client.ExitError
	}

	excludeOrgs := parseCSV(*excludeOrgsFlag)

	fmt.Fprintf(os.Stderr, "Fetching Copilot seats for enterprise %q... ", enterprise)
	seats, totalSeats, err := copilot.ListSeats(c, enterprise)
	if err != nil {
		return handleError(err)
	}
	fmt.Fprintf(os.Stderr, "OK (%d seats)\n", len(seats))

	result := copilot.Analyze(seats, excludeOrgs)

	// --expect-count guard.
	if *expectCount > 0 && result.DistinctEnterprise != *expectCount {
		fmt.Fprintf(os.Stderr,
			"\nError: expect-count mismatch.\n"+
				"  Expected distinct Enterprise users : %d\n"+
				"  Actual distinct Enterprise users   : %d\n"+
				"  Investigate before proceeding. To disable: --expect-count=0\n",
			*expectCount, result.DistinctEnterprise)
		return client.ExitHealthCheck
	}

	if format == "json" {
		return jsonOut(copilot.AuditJSON{
			Enterprise:         enterprise,
			TotalSeats:         totalSeats,
			Classification:     result.Classification,
			TotalEnterprise:    result.TotalEnterprise,
			DistinctEnterprise: result.DistinctEnterprise,
			CrossOrgUsers:      result.CrossOrgUsers,
			ByOrg:              result.ByOrg,
			BusinessSeats:      result.BusinessSeats,
		})
	}

	report.AuditText(os.Stdout, enterprise, totalSeats, result, excludeOrgs)

	if *strict && len(result.BusinessSeats) > 0 {
		return client.ExitHealthCheck
	}
	return client.ExitOK
}

// ---- billing --------------------------------------------------------------

func cmdBilling(c *client.Client, enterprise, format string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: 'billing' requires a sub-subcommand: usage | summary | premium | credits | budgets | report")
		return client.ExitError
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "usage":
		return cmdBillingUsage(c, enterprise, format, rest)
	case "summary":
		return cmdBillingUsageSummary(c, enterprise, format, rest)
	case "premium":
		return cmdBillingPremium(c, enterprise, format, rest)
	case "credits":
		return cmdBillingCredits(c, enterprise, format, rest)
	case "budgets":
		return cmdBillingBudgets(c, enterprise, format, rest)
	case "report":
		return cmdBillingReport(c, enterprise, format, rest)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown billing subcommand %q\n", sub)
		fmt.Fprintln(os.Stderr, "Valid: usage | summary | premium | credits | budgets | report")
		return client.ExitError
	}
}

func cmdBillingUsage(c *client.Client, enterprise, format string, args []string) int {
	return runBillingCmd(c, enterprise, format, args, parseUsageFilter,
		"Fetching billing usage for enterprise %q...\n",
		billing.GetUsageJSON, billing.GetUsage, report.BillingUsageText)
}

func cmdBillingUsageSummary(c *client.Client, enterprise, format string, args []string) int {
	return runBillingCmd(c, enterprise, format, args, parseUsageFilter,
		"Fetching billing usage summary for enterprise %q...\n",
		billing.GetUsageSummaryJSON, billing.GetUsageSummary, report.BillingUsageSummaryText)
}

func cmdBillingPremium(c *client.Client, enterprise, format string, args []string) int {
	return runBillingCmd(c, enterprise, format, args, parseConsumptionFilter,
		"Fetching premium-request usage for enterprise %q...\n",
		billing.GetPremiumRequestUsageJSON, billing.GetPremiumRequestUsage,
		func(w io.Writer, rep *billing.ConsumptionReport) { report.ConsumptionText(w, "Premium Request", rep) })
}

func cmdBillingCredits(c *client.Client, enterprise, format string, args []string) int {
	return runBillingCmd(c, enterprise, format, args, parseConsumptionFilter,
		"Fetching AI-credit usage for enterprise %q...\n",
		billing.GetAICreditUsageJSON, billing.GetAICreditUsage,
		func(w io.Writer, rep *billing.ConsumptionReport) { report.ConsumptionText(w, "AI Credit", rep) })
}

func cmdBillingBudgets(c *client.Client, enterprise, format string, args []string) int {
	return runBillingCmd(c, enterprise, format, args, parseBudgetFilter,
		"Fetching budgets for enterprise %q...\n",
		billing.ListBudgetsJSON, billing.ListBudgets, report.BudgetsText)
}

// cmdBillingReport does not use runBillingCmd: it needs to inject time.Now()
// for GetPeriodComparison and its return type isn't a simple filter->value
// fetch, so it is written out like cmdAudit/cmdMetricsReport.
func cmdBillingReport(c *client.Client, enterprise, format string, args []string) int {
	f, err := parsePeriodReportFilter(args)
	if err != nil {
		if isHelpErr(err) {
			return client.ExitOK
		}
		return client.ExitError
	}

	fmt.Fprintf(os.Stderr,
		"Fetching two-period billing report (previous closed month + month-to-date) for enterprise %q...\n",
		enterprise)

	pc, err := billing.GetPeriodComparison(c, enterprise, f, time.Now().UTC())
	if err != nil {
		return handleError(err)
	}

	if format == "json" {
		return jsonOut(pc)
	}
	report.BillingPeriodReportText(os.Stdout, pc)
	return client.ExitOK
}

// runBillingCmd is the shape shared by every `billing <sub>` command: parse
// its filter flags, print a fetch banner, then either fetch raw JSON
// (--format json) or a typed value and render it as text.
func runBillingCmd[F, T any](
	c *client.Client, enterprise, format string, args []string,
	parseFilter func([]string) (F, error),
	banner string,
	fetchJSON func(*client.Client, string, F) (json.RawMessage, error),
	fetch func(*client.Client, string, F) (T, error),
	render func(io.Writer, T),
) int {
	f, err := parseFilter(args)
	if err != nil {
		if isHelpErr(err) {
			return client.ExitOK
		}
		return client.ExitError
	}
	fmt.Fprintf(os.Stderr, banner, enterprise)

	if format == "json" {
		raw, err := fetchJSON(c, enterprise, f)
		if err != nil {
			return handleError(err)
		}
		return jsonOut(raw)
	}
	v, err := fetch(c, enterprise, f)
	if err != nil {
		return handleError(err)
	}
	render(os.Stdout, v)
	return client.ExitOK
}

// ---- metrics --------------------------------------------------------------

func cmdMetrics(c *client.Client, enterprise, format string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: 'metrics' requires a sub-subcommand: report")
		return client.ExitError
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "report":
		return cmdMetricsReport(c, enterprise, format, rest)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown metrics subcommand %q\n", sub)
		return client.ExitError
	}
}

func cmdMetricsReport(c *client.Client, enterprise, format string, args []string) int {
	fs := flag.NewFlagSet("metrics report", flag.ContinueOnError)
	scopeFlag := fs.String("scope", string(metrics.ScopeEnterprise28Day),
		"Report scope: enterprise-28-day | enterprise-1-day | users-28-day | users-1-day | user-teams-1-day")
	day := fs.String("day", "", "Day for 1-day reports (YYYY-MM-DD); defaults to yesterday")
	download := fs.Bool("download", false, "Download the report files to --output-dir")
	outputDir := fs.String("output-dir", ".", "Directory to save downloaded report files")
	if err := fs.Parse(args); err != nil {
		if isHelpErr(err) {
			return client.ExitOK
		}
		return client.ExitError
	}

	scope := metrics.ReportScope(*scopeFlag)
	if !metrics.ValidScope(scope) {
		fmt.Fprintf(os.Stderr, "Error: invalid --scope %q\nValid: %s\n",
			*scopeFlag, strings.Join(metrics.ScopeNames(), " | "))
		return client.ExitError
	}

	// Default day = yesterday for 1-day scopes.
	if *day == "" && strings.HasSuffix(string(scope), "-1-day") {
		*day = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}

	fmt.Fprintf(os.Stderr, "Fetching Copilot metrics report (scope=%s) for enterprise %q...\n", scope, enterprise)

	if format == "json" {
		raw, err := metrics.GetReportLinksJSON(c, enterprise, scope, *day)
		if err != nil {
			return handleError(err)
		}
		return jsonOut(raw)
	}

	links, err := metrics.GetReportLinks(c, enterprise, scope, *day)
	if err != nil {
		return handleError(err)
	}
	report.MetricsReportText(os.Stdout, scope, links)

	if *download {
		fmt.Fprintf(os.Stderr, "Downloading %d file(s) to %q...\n", len(links.DownloadLinks), *outputDir)
		paths, err := metrics.Download(links, *outputDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error downloading: %v\n", err)
			return client.ExitError
		}
		report.MetricsDownloadedText(os.Stdout, paths)
	}
	return client.ExitOK
}

// ---- flag helpers ----------------------------------------------------------

// commonFilter holds the flag values shared by every usage/consumption
// endpoint (year/month/day/org/product/cost-center).
type commonFilter struct {
	year, month, day     *int
	org, product, costCt *string
}

// registerCommonFilterFlags registers the shared flags onto fs.
func registerCommonFilterFlags(fs *flag.FlagSet) commonFilter {
	return commonFilter{
		year:    fs.Int("year", 0, "Year (YYYY)"),
		month:   fs.Int("month", 0, "Month (1-12)"),
		day:     fs.Int("day", 0, "Day (1-31)"),
		org:     fs.String("org", "", "Filter by organization"),
		product: fs.String("product", "", "Filter by product (e.g. copilot)"),
		costCt:  fs.String("cost-center", "", "Filter by cost center ID"),
	}
}

func parseUsageFilter(args []string) (billing.UsageFilter, error) {
	fs := flag.NewFlagSet("usage-filter", flag.ContinueOnError)
	cf := registerCommonFilterFlags(fs)
	sku := fs.String("sku", "", "Filter by SKU")
	if err := fs.Parse(args); err != nil {
		return billing.UsageFilter{}, err
	}
	return billing.UsageFilter{
		Year:         *cf.year,
		Month:        *cf.month,
		Day:          *cf.day,
		Organization: *cf.org,
		Product:      *cf.product,
		SKU:          *sku,
		CostCenterID: *cf.costCt,
	}, nil
}

func parseConsumptionFilter(args []string) (billing.ConsumptionFilter, error) {
	fs := flag.NewFlagSet("consumption-filter", flag.ContinueOnError)
	cf := registerCommonFilterFlags(fs)
	user := fs.String("user", "", "Filter by user login")
	model := fs.String("model", "", "Filter by model name")
	if err := fs.Parse(args); err != nil {
		return billing.ConsumptionFilter{}, err
	}
	return billing.ConsumptionFilter{
		Year:         *cf.year,
		Month:        *cf.month,
		Day:          *cf.day,
		Organization: *cf.org,
		User:         *user,
		Model:        *model,
		Product:      *cf.product,
		CostCenterID: *cf.costCt,
	}, nil
}

func parseBudgetFilter(args []string) (billing.BudgetFilter, error) {
	fs := flag.NewFlagSet("billing-budgets-filter", flag.ContinueOnError)
	scope := fs.String("scope", "", "Filter by scope: enterprise|organization|repository|cost_center|user")
	user := fs.String("user", "", "Filter by user login")
	if err := fs.Parse(args); err != nil {
		return billing.BudgetFilter{}, err
	}
	return billing.BudgetFilter{Scope: *scope, User: *user}, nil
}

// parsePeriodReportFilter registers only org/product/cost-center for
// `billing report` — there is deliberately no --year/--month/--day: the
// whole point of this subcommand is to auto-select the previous closed
// month and the current month-to-date, so exposing those would invite
// confusing, contradictory combinations.
func parsePeriodReportFilter(args []string) (billing.UsageFilter, error) {
	fs := flag.NewFlagSet("billing-report-filter", flag.ContinueOnError)
	org := fs.String("org", "", "Filter by organization")
	product := fs.String("product", "", "Filter by product (e.g. copilot)")
	costCt := fs.String("cost-center", "", "Filter by cost center ID")
	if err := fs.Parse(args); err != nil {
		return billing.UsageFilter{}, err
	}
	return billing.UsageFilter{
		Organization: *org,
		Product:      *product,
		CostCenterID: *costCt,
	}, nil
}

func parseCSV(s string) map[string]bool {
	m := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			m[t] = true
		}
	}
	return m
}

// parseSubcommand splits the remaining args into (command, rest). The
// command is always a single word (e.g. "billing", "metrics"); each of
// those in turn dispatches on rest[0] for its own sub-subcommand (e.g.
// "usage", "report") in cmdBilling/cmdMetrics.
func parseSubcommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}

// isHelpErr reports whether err is flag.ErrHelp, i.e. -h/--help was
// requested and its FlagSet already printed usage. Callers use this to
// exit 0 (success) instead of 1 (a real parse error) for --help, matching
// standard Unix CLI convention.
func isHelpErr(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

// ---- output helpers --------------------------------------------------------

// jsonOut encodes v as indented JSON to stdout. v may be a plain value (e.g.
// a map built for --format json) or a json.RawMessage passed straight
// through from an API response — both are handled correctly since
// json.RawMessage implements json.Marshaler.
func jsonOut(v any) int {
	if err := report.JSON(os.Stdout, v); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing JSON:", err)
		return client.ExitError
	}
	return client.ExitOK
}

// handleError prints err (its full text, including any wrapping context
// like "previous month (2026-06): ...") and returns the appropriate exit
// code. Uses errors.As (not a direct type assertion) so it also correctly
// unwraps a *client.Error that's been wrapped by fmt.Errorf("...: %w", err),
// e.g. by billing.GetPeriodComparison.
func handleError(err error) int {
	fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	var ce *client.Error
	if errors.As(err, &ce) {
		return ce.ExitCode
	}
	return client.ExitError
}

// ---- usage -----------------------------------------------------------------

func printUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stderr, `copilot-license-audit %s

Audit GitHub Copilot Enterprise seat assignments and report billing usage.
All operations are strictly read-only — no API write calls are ever made.

USAGE
  copilot-license-audit --enterprise <slug> [subcommand] [flags]

GLOBAL FLAGS
`, version)
	fs.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
SUBCOMMANDS
  audit                      Seat inventory + post-migration health check (default)
    --exclude-orgs <csv>     Org names to omit from the inventory
    --strict                 Exit 2 if any Business seats are detected
    --expect-count <n>       Expected distinct Enterprise users (0=disabled)

  billing usage              Enhanced billing platform usage line items
  billing summary            Aggregated billing usage summary
  billing premium            Premium-request consumption report
  billing credits            AI-credit consumption report
  billing budgets            List enterprise budgets
    Shared billing flags:
    --year <n>               Year (default: current)
    --month <n>              Month (default: current)
    --day <n>                Day
    --org <name>             Filter by organization
    --product <name>         Filter by product (e.g. copilot)
    --cost-center <id>       Filter by cost center ID

  billing report             Two-period report: previous closed month +
                             current month-to-date (Billing Summary,
                             Premium Requests, and AI Credits for each)
    --org <name>             Filter by organization
    --product <name>         Filter by product (e.g. copilot)
    --cost-center <id>       Filter by cost center ID
    (no --year/--month/--day — both periods are computed automatically)

  metrics report             Copilot Enterprise usage metrics report
    --scope <s>              Report scope (default: enterprise-28-day)
                             enterprise-28-day | enterprise-1-day |
                             users-28-day | users-1-day | user-teams-1-day
    --day <YYYY-MM-DD>       Day for 1-day scopes (default: yesterday)
    --download               Download the report files
    --output-dir <path>      Directory for downloaded files (default: .)

EXAMPLES
  # Post-migration health check (assert Business seats == 0):
  copilot-license-audit --enterprise my-enterprise audit --strict
  copilot-license-audit --enterprise my-enterprise audit --expect-count 500
  copilot-license-audit --enterprise my-enterprise audit --exclude-orgs my-sandbox-org

  # Enhanced billing usage, filtered to Copilot, current month:
  copilot-license-audit --enterprise my-enterprise billing usage --product copilot

  # Aggregated billing summary for a specific month:
  copilot-license-audit --enterprise my-enterprise billing summary --year 2026 --month 5

  # Premium request consumption:
  copilot-license-audit --enterprise my-enterprise billing premium

  # AI credit consumption, JSON output:
  copilot-license-audit --enterprise my-enterprise billing credits --format json

  # Budgets:
  copilot-license-audit --enterprise my-enterprise billing budgets

  # Two-period report (previous closed month + current month-to-date):
  copilot-license-audit --enterprise my-enterprise billing report
  copilot-license-audit --enterprise my-enterprise billing report --product copilot

  # Latest 28-day Copilot Enterprise metrics report:
  copilot-license-audit --enterprise my-enterprise metrics report

  # Per-user metrics for a specific day, with download:
  copilot-license-audit --enterprise my-enterprise metrics report \
    --scope users-1-day --day 2026-06-25 --download --output-dir ./reports

EXIT CODES
  0  Success
  1  API or authentication error
  2  Health check failed (--strict) or --expect-count mismatch

PREREQUISITES
  Provide a GitHub token via one of:
    - $GH_TOKEN or $GITHUB_TOKEN (any valid token, e.g. a classic PAT —
      useful in CI or anywhere you'd rather not install gh), or
    - the GitHub CLI (https://cli.github.com): gh auth login

  Whichever you use, it needs these scopes: manage_billing:copilot, read:enterprise

  If you're using the GitHub CLI, refresh them with:
    %s

  Required role: Enterprise Owner or Billing Manager on the target enterprise.
`, client.AuthRefreshHint)
}
