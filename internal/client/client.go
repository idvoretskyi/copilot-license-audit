// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package client provides a thin GitHub REST API client: bearer auth,
// X-GitHub-Api-Version, Link-header pagination, 429/5xx retry, and rate-limit
// reset waiting.  Zero third-party dependencies — stdlib net/http only.
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIVersion     = "2026-03-10"
	DefaultBaseURL        = "https://api.github.com"
	maxRetries            = 3
	rateLimitFallbackWait = 60 * time.Second
)

var linkNextRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Client is a minimal GitHub REST client.
type Client struct {
	token      string
	apiVersion string
	baseURL    string
	verbose    bool
	http       *http.Client
}

// New constructs a Client.  baseURL defaults to https://api.github.com.
func New(token, apiVersion, baseURL string, verbose bool) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	return &Client{
		token:      token,
		apiVersion: apiVersion,
		baseURL:    strings.TrimRight(baseURL, "/"),
		verbose:    verbose,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// URL builds a full URL from a path relative to the base URL.
func (c *Client) URL(path string) string {
	return c.baseURL + path
}

// GetJSON performs a GET and decodes the JSON response body into dst.
// It handles 429 and 5xx retries automatically.
func (c *Client) GetJSON(url string, dst any) error {
	body, _, err := c.do(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// Get performs a GET and decodes the JSON response body into a new T.
// It is a generic convenience wrapper around GetJSON, used by callers that
// would otherwise repeat "declare zero value, GetJSON, return &value" for
// every typed endpoint.
func Get[T any](c *Client, url string) (*T, error) {
	var v T
	if err := c.GetJSON(url, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// GetRaw performs a GET and returns the response body as a json.RawMessage,
// for callers that need to pass through unparsed JSON (e.g. --format json).
func GetRaw(c *Client, url string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.GetJSON(url, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// GetJSONPaged calls fn for every page of results, following Link rel=next.
// fn receives the raw JSON body; pagination stops when there is no next link.
func (c *Client) GetJSONPaged(firstURL string, fn func(body []byte) error) error {
	url := firstURL
	page := 1
	for url != "" {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] GET page %d: %s\n", page, url)
		}
		body, headers, err := c.do(url)
		if err != nil {
			return err
		}
		if err := fn(body); err != nil {
			return err
		}
		url = extractNextLink(headers.Get("Link"))
		page++
	}
	return nil
}

// do executes a GET request with retries. Returns (body, responseHeaders, err).
//
// This client is intentionally GET-only: do never accepts a method or
// request body, which makes the tool's "strictly read-only, no API write
// calls are ever made" guarantee structurally true rather than just a
// convention every call site has to honor.
func (c *Client) do(url string) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 && c.verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] Retry %d/%d  GET %s\n", attempt, maxRetries, url)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, nil, APIError("building request: %v", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("X-GitHub-Api-Version", c.apiVersion)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			c.backoff(attempt)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, nil, APIError("reading response body: %v", readErr)
		}

		switch {
		case resp.StatusCode == 200 || resp.StatusCode == 204:
			return body, resp.Header, nil

		case resp.StatusCode == 401:
			return nil, nil, AuthError(
				"401 Unauthorized — token invalid or expired.\n\n" +
					"  " + AuthRefreshHint)

		case resp.StatusCode == 403:
			return nil, nil, AuthError(
				"403 Forbidden — missing permissions or scope.\n\n" +
					"  • Ensure the account is an Enterprise Owner or Billing Manager.\n" +
					"  • " + AuthRefreshHint + "\n" +
					"  • gh auth status")

		case resp.StatusCode == 404:
			return nil, nil, APIError(
				"404 Not Found — enterprise or resource not found.\n\n" +
					"  • Verify the --enterprise slug.\n" +
					"  • Confirm Copilot is enabled for the enterprise.\n" +
					"  • Confirm Enterprise Owner or Billing Manager role.\n" +
					"  • " + AuthRefreshHint)

		case resp.StatusCode == 429:
			if attempt == maxRetries {
				lastErr = fmt.Errorf("rate limited after %d retries", maxRetries)
				break
			}
			c.waitRateLimit(resp.Header)
			continue

		case resp.StatusCode >= 500:
			if attempt == maxRetries {
				lastErr = fmt.Errorf("server error %d", resp.StatusCode)
				break
			}
			c.backoff(attempt)
			continue

		default:
			return nil, nil, APIError("unexpected status %d for %s: %s",
				resp.StatusCode, url, strings.TrimSpace(string(body)))
		}
	}
	return nil, nil, APIError("request to %s failed after %d retries: %v", url, maxRetries, lastErr)
}

func (c *Client) waitRateLimit(h http.Header) {
	wait := rateLimitFallbackWait
	if reset := h.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			// If reset is in the past (or now), wait 0; otherwise wait until reset + 1s buffer.
			if w := time.Until(time.Unix(ts, 0)) + time.Second; w <= 0 {
				wait = 0
			} else {
				wait = w
			}
		}
	}
	if c.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Rate limited — sleeping %.0fs\n", wait.Seconds())
	} else {
		fmt.Fprintf(os.Stderr, "  Rate limited — waiting %.0fs for reset...\n", wait.Seconds())
	}
	time.Sleep(wait)
}

func (c *Client) backoff(attempt int) {
	wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if c.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Backing off %.0fs\n", wait.Seconds())
	}
	time.Sleep(wait)
}

func extractNextLink(linkHeader string) string {
	if m := linkNextRE.FindStringSubmatch(linkHeader); len(m) == 2 {
		return m[1]
	}
	return ""
}
