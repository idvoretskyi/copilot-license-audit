// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package metrics provides access to the Copilot Enterprise usage metrics
// report endpoints — the "Copilot Enterprise billing reporting option".
//
// Each endpoint returns signed download URLs (not the data inline).
// Use the --download flag in the CLI to fetch and save the linked files.
package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// ReportScope selects which report series to fetch.
type ReportScope string

const (
	ScopeEnterprise28Day ReportScope = "enterprise-28-day" // latest 28-day aggregate
	ScopeEnterprise1Day  ReportScope = "enterprise-1-day"  // single day aggregate
	ScopeUsers28Day      ReportScope = "users-28-day"      // latest 28-day per-user
	ScopeUsers1Day       ReportScope = "users-1-day"       // single day per-user
	ScopeUserTeams1Day   ReportScope = "user-teams-1-day"  // user-team join for a day
)

// AllScopes lists every supported ReportScope value.
var AllScopes = []ReportScope{
	ScopeEnterprise28Day,
	ScopeEnterprise1Day,
	ScopeUsers28Day,
	ScopeUsers1Day,
	ScopeUserTeams1Day,
}

// ValidScope reports whether scope is one of AllScopes.
func ValidScope(scope ReportScope) bool {
	for _, s := range AllScopes {
		if scope == s {
			return true
		}
	}
	return false
}

// ScopeNames returns AllScopes as their underlying strings, in order —
// convenient for building "valid values" help/error messages.
func ScopeNames() []string {
	names := make([]string, len(AllScopes))
	for i, s := range AllScopes {
		names[i] = string(s)
	}
	return names
}

// ReportLinks is the response from a metrics report endpoint.
type ReportLinks struct {
	DownloadLinks []string `json:"download_links"`
	// 28-day reports include a date range; 1-day reports include a single day.
	ReportDay      string `json:"report_day,omitempty"`
	ReportStartDay string `json:"report_start_day,omitempty"`
	ReportEndDay   string `json:"report_end_day,omitempty"`
}

// GetReportLinks fetches the signed download links for a Copilot Enterprise
// usage metrics report.
//
// For ScopeEnterprise28Day and ScopeUsers28Day the "latest" endpoint is used.
// For *1Day scopes a specific day (YYYY-MM-DD) must be provided via day.
func GetReportLinks(c *client.Client, enterprise string, scope ReportScope, day string) (*ReportLinks, error) {
	return client.Get[ReportLinks](c, reportURL(c, enterprise, scope, day))
}

// GetReportLinksJSON returns raw JSON for --format json.
func GetReportLinksJSON(c *client.Client, enterprise string, scope ReportScope, day string) (json.RawMessage, error) {
	return client.GetRaw(c, reportURL(c, enterprise, scope, day))
}

// Download fetches each URL in links and writes the files to dir.
// Files are named after the last path segment of the URL (before the query string).
// Returns a list of file paths written.
func Download(links *ReportLinks, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory %q: %w", dir, err)
	}
	var paths []string
	for _, dl := range links.DownloadLinks {
		name := urlBasename(dl)
		dest := filepath.Join(dir, name)
		if err := downloadFile(dl, dest); err != nil {
			return paths, fmt.Errorf("downloading %s: %w", name, err)
		}
		paths = append(paths, dest)
	}
	return paths, nil
}

// reportURL constructs the correct API URL for the given scope and day.
func reportURL(c *client.Client, enterprise string, scope ReportScope, day string) string {
	base := c.URL(fmt.Sprintf("/enterprises/%s/copilot/metrics/reports", enterprise))
	switch scope {
	case ScopeEnterprise28Day:
		return base + "/enterprise-28-day/latest"
	case ScopeUsers28Day:
		return base + "/users-28-day/latest"
	case ScopeEnterprise1Day:
		return withDay(base+"/enterprise-1-day", day)
	case ScopeUsers1Day:
		return withDay(base+"/users-1-day", day)
	case ScopeUserTeams1Day:
		return withDay(base+"/user-teams-1-day", day)
	default:
		return fmt.Sprintf("%s/%s", base, scope)
	}
}

// withDay appends a properly-escaped ?day= query parameter.
func withDay(base, day string) string {
	q := url.Values{}
	q.Set("day", day)
	return base + "?" + q.Encode()
}

// StripQuery removes everything from the first '?' onward, if present.
// Shared by urlBasename (extracting a filename from a signed download URL)
// and by the text report renderer (shortening a signed URL for display).
func StripQuery(rawURL string) string {
	if i := strings.IndexByte(rawURL, '?'); i >= 0 {
		return rawURL[:i]
	}
	return rawURL
}

// urlBasename extracts a safe filename from a signed download URL.
func urlBasename(rawURL string) string {
	return filepath.Base(StripQuery(rawURL))
}

// downloadHTTPClient is used only for fetching the signed report files
// themselves (metrics.Download), separate from the api.github.com client —
// these URLs point at a different host and need no auth headers, just a
// bounded timeout so a stalled download can't hang the process forever.
var downloadHTTPClient = &http.Client{Timeout: 2 * time.Minute}

// downloadFile saves a remote URL to a local file path.
func downloadFile(url, dest string) error {
	resp, err := downloadHTTPClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from download URL", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
