// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

// Package copilot fetches Copilot seat data from the GitHub Enterprise API.
package copilot

import (
	"encoding/json"
	"fmt"

	"github.com/idvoretskyi/copilot-license-audit/internal/client"
)

// PlanType constants mirror the GitHub API enum (business | enterprise | unknown).
const (
	PlanBusiness   = "business"
	PlanEnterprise = "enterprise"
	PlanUnknown    = "unknown"
)

// Seat represents a single Copilot seat assignment.
type Seat struct {
	Login                   string `json:"login"`
	Org                     string `json:"org"`
	PlanType                string `json:"plan_type"`                           // "business" | "enterprise" | "unknown" | ""
	PendingCancellationDate string `json:"pending_cancellation_date,omitempty"` // YYYY-MM-DD or ""
	LastActivityAt          string `json:"last_activity_at,omitempty"`          // RFC3339 or ""
	LastActivityEditor      string `json:"last_activity_editor,omitempty"`
	LastAuthenticatedAt     string `json:"last_authenticated_at,omitempty"` // RFC3339 or ""
	CreatedAt               string `json:"created_at,omitempty"`            // RFC3339
}

// apiSeat is the raw JSON shape returned by the GitHub API.
type apiSeat struct {
	Assignee struct {
		Login string `json:"login"`
	} `json:"assignee"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
	PendingCancellationDate string `json:"pending_cancellation_date"`
	LastActivityAt          string `json:"last_activity_at"`
	LastActivityEditor      string `json:"last_activity_editor"`
	LastAuthenticatedAt     string `json:"last_authenticated_at"`
	CreatedAt               string `json:"created_at"`
	PlanType                string `json:"plan_type"`
}

type apiPage struct {
	TotalSeats int       `json:"total_seats"`
	Seats      []apiSeat `json:"seats"`
}

// ListSeats fetches all Copilot seat assignments for the enterprise (paginated).
// Returns (seats, total_seats_reported_by_api, error).
//
// Endpoint: GET /enterprises/{enterprise}/copilot/billing/seats
// Required scope: manage_billing:copilot or read:enterprise
func ListSeats(c *client.Client, enterprise string) ([]Seat, int, error) {
	firstURL := c.URL(fmt.Sprintf(
		"/enterprises/%s/copilot/billing/seats?per_page=100&page=1", enterprise,
	))

	var seats []Seat
	totalSeats := 0
	firstPage := true

	err := c.GetJSONPaged(firstURL, func(body []byte) error {
		var page apiPage
		if err := json.Unmarshal(body, &page); err != nil {
			return client.APIError("parsing seats page: %v", err)
		}
		if firstPage {
			totalSeats = page.TotalSeats
			firstPage = false
		}
		for _, s := range page.Seats {
			login := s.Assignee.Login
			org := s.Organization.Login
			if login == "" || org == "" {
				continue
			}
			seats = append(seats, Seat{
				Login:                   login,
				Org:                     org,
				PlanType:                s.PlanType,
				PendingCancellationDate: s.PendingCancellationDate,
				LastActivityAt:          s.LastActivityAt,
				LastActivityEditor:      s.LastActivityEditor,
				LastAuthenticatedAt:     s.LastAuthenticatedAt,
				CreatedAt:               s.CreatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return seats, totalSeats, nil
}
