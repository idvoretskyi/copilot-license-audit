// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package metrics_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
	"github.com/idvoretskyi/copilot-license-audit/internal/metrics"
)

func newClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	return client.New("tok", client.DefaultAPIVersion, baseURL, false)
}

// TestGetReportLinks_28day verifies the latest 28-day endpoint path.
func TestGetReportLinks_28day(t *testing.T) {
	resp := metrics.ReportLinks{
		DownloadLinks:  []string{"https://example.com/report.parquet?sig=abc"},
		ReportStartDay: "2026-05-30",
		ReportEndDay:   "2026-06-26",
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/enterprises/test-ent/copilot/metrics/reports/enterprise-28-day/latest"
		if r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		w.Write(body)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	got, err := metrics.GetReportLinks(c, "test-ent", metrics.ScopeEnterprise28Day, "")
	if err != nil {
		t.Fatalf("GetReportLinks error: %v", err)
	}
	if got.ReportStartDay != "2026-05-30" {
		t.Errorf("ReportStartDay = %q, want 2026-05-30", got.ReportStartDay)
	}
	if len(got.DownloadLinks) != 1 {
		t.Errorf("DownloadLinks count = %d, want 1", len(got.DownloadLinks))
	}
}

// TestGetReportLinks_1day verifies the 1-day endpoint appends ?day=.
func TestGetReportLinks_1day(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/enterprises/test-ent/copilot/metrics/reports/enterprise-1-day" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("day"); got != "2026-06-25" {
			t.Errorf("day query = %q, want 2026-06-25", got)
		}
		fmt.Fprint(w, `{"download_links":[],"report_day":"2026-06-25"}`)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	_, err := metrics.GetReportLinks(c, "test-ent", metrics.ScopeEnterprise1Day, "2026-06-25")
	if err != nil {
		t.Fatalf("GetReportLinks error: %v", err)
	}
}

// TestGetReportLinks_usersScopes verifies user-level scope paths.
func TestGetReportLinks_usersScopes(t *testing.T) {
	cases := []struct {
		scope metrics.ReportScope
		want  string
	}{
		{metrics.ScopeUsers28Day, "/enterprises/e/copilot/metrics/reports/users-28-day/latest"},
		{metrics.ScopeUsers1Day, "/enterprises/e/copilot/metrics/reports/users-1-day"},
		{metrics.ScopeUserTeams1Day, "/enterprises/e/copilot/metrics/reports/user-teams-1-day"},
	}
	for _, tc := range cases {
		t.Run(string(tc.scope), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.want {
					t.Errorf("path = %q, want %q", r.URL.Path, tc.want)
				}
				fmt.Fprint(w, `{"download_links":[],"report_day":"2026-06-25"}`)
			}))
			defer srv.Close()
			c := newClient(t, srv.URL)
			_, err := metrics.GetReportLinks(c, "e", tc.scope, "2026-06-25")
			if err != nil {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

// TestDownload verifies that report files are written to disk.
func TestDownload(t *testing.T) {
	content := []byte("report-data")
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(content)
	}))
	defer fileSrv.Close()

	links := &metrics.ReportLinks{
		DownloadLinks: []string{fileSrv.URL + "/report.parquet"},
	}

	dir := t.TempDir()
	paths, err := metrics.Download(links, dir)
	if err != nil {
		t.Fatalf("Download error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths count = %d, want 1", len(paths))
	}
	got, err := os.ReadFile(filepath.Join(dir, "report.parquet"))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", got, content)
	}
}
