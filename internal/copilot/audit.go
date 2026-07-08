// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Ihor Dvoretskyi

package copilot

import "sort"

// AuditResult contains the classified and grouped seat data.
type AuditResult struct {
	// Classification is a map of plan_type → count across all seats fetched.
	Classification map[string]int

	// ByOrg maps org login → sorted list of logins for Enterprise-plan seats.
	ByOrg map[string][]string

	// TotalEnterprise is the number of Enterprise-plan seat rows.
	TotalEnterprise int

	// DistinctEnterprise is the count of unique logins on Enterprise plan.
	DistinctEnterprise int

	// BusinessSeats contains any stray Business-plan seats (should be empty
	// post-migration).  Non-empty triggers the --strict health-check alarm.
	BusinessSeats []Seat

	// CrossOrgUsers is the list of logins assigned in more than one org.
	CrossOrgUsers []string
}

// AuditJSON is the --format json shape for the `audit` subcommand, mirroring
// AuditResult plus the raw seat count. It exists as a named, tagged type
// (rather than an ad-hoc map built in main.go) so the JSON output shape is
// documented and typo-proof in one place, matching every other subcommand's
// --format json path.
type AuditJSON struct {
	Enterprise         string              `json:"enterprise"`
	TotalSeats         int                 `json:"total_seats"`
	Classification     map[string]int      `json:"classification"`
	TotalEnterprise    int                 `json:"total_enterprise"`
	DistinctEnterprise int                 `json:"distinct_enterprise"`
	CrossOrgUsers      []string            `json:"cross_org_users"`
	ByOrg              map[string][]string `json:"by_org"`
	BusinessSeats      []Seat              `json:"business_seats"`
}

// Analyze classifies seats and produces an AuditResult.
// excludeOrgs contains org logins to omit from ByOrg (e.g. already handled).
func Analyze(seats []Seat, excludeOrgs map[string]bool) AuditResult {
	classification := map[string]int{}
	enterpriseGroups := map[string]map[string]struct{}{} // org → set of logins
	loginOrgs := map[string]map[string]struct{}{}        // login → set of distinct orgs (for cross-org detection)
	var businessSeats []Seat

	for _, s := range seats {
		key := s.PlanType
		if key == "" {
			key = "(empty)"
		}
		classification[key]++

		switch s.PlanType {
		case PlanEnterprise:
			if excludeOrgs[s.Org] {
				continue
			}
			if _, ok := enterpriseGroups[s.Org]; !ok {
				enterpriseGroups[s.Org] = map[string]struct{}{}
			}
			enterpriseGroups[s.Org][s.Login] = struct{}{}
			if _, ok := loginOrgs[s.Login]; !ok {
				loginOrgs[s.Login] = map[string]struct{}{}
			}
			loginOrgs[s.Login][s.Org] = struct{}{}

		case PlanBusiness:
			businessSeats = append(businessSeats, s)
		}
	}

	// Build sorted per-org slices.
	byOrg := make(map[string][]string, len(enterpriseGroups))
	totalEnterprise := 0
	for org, logins := range enterpriseGroups {
		sl := make([]string, 0, len(logins))
		for l := range logins {
			sl = append(sl, l)
		}
		sort.Strings(sl)
		byOrg[org] = sl
		totalEnterprise += len(sl)
	}

	distinctEnterprise := len(loginOrgs)

	// Cross-org: logins that appear in more than one *distinct* org.
	var crossOrg []string
	for login, orgs := range loginOrgs {
		if len(orgs) > 1 {
			crossOrg = append(crossOrg, login)
		}
	}
	sort.Strings(crossOrg)
	sort.Slice(businessSeats, func(i, j int) bool {
		if businessSeats[i].Org != businessSeats[j].Org {
			return businessSeats[i].Org < businessSeats[j].Org
		}
		return businessSeats[i].Login < businessSeats[j].Login
	})

	return AuditResult{
		Classification:     classification,
		ByOrg:              byOrg,
		TotalEnterprise:    totalEnterprise,
		DistinctEnterprise: distinctEnterprise,
		BusinessSeats:      businessSeats,
		CrossOrgUsers:      crossOrg,
	}
}

// SortedPlanKeys returns plan_type keys in a stable display order:
// enterprise first, business second, then alphabetical.
func SortedPlanKeys(m map[string]int) []string {
	priority := map[string]int{PlanEnterprise: 0, PlanBusiness: 1}
	rankOf := func(k string) int {
		if r, ok := priority[k]; ok {
			return r
		}
		return 99 // unrecognised values sort last
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := rankOf(keys[i]), rankOf(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// SortedOrgs returns org names sorted descending by seat count, then alpha.
func SortedOrgs(byOrg map[string][]string) []string {
	orgs := make([]string, 0, len(byOrg))
	for o := range byOrg {
		orgs = append(orgs, o)
	}
	sort.Slice(orgs, func(i, j int) bool {
		li, lj := len(byOrg[orgs[i]]), len(byOrg[orgs[j]])
		if li != lj {
			return li > lj
		}
		return orgs[i] < orgs[j]
	})
	return orgs
}
