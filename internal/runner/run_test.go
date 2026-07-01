// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// fakeGsuiteClient returns canned groups or an error per domain.
type fakeGsuiteClient struct {
	groupsByDomain map[string][]string
	errByDomain    map[string]error
}

func (f *fakeGsuiteClient) GetGroupsFromUser(domain string, _ string) ([]string, error) {
	if err := f.errByDomain[domain]; err != nil {
		return nil, err
	}
	return f.groupsByDomain[domain], nil
}

// getGsuiteGroupsForUser must union the user's groups across every configured domain and deduplicate them.
func TestGetGsuiteGroupsForUserUnionsAndDeduplicates(t *testing.T) {
	tests := map[string]struct {
		domains        []string
		groupsByDomain map[string][]string
		want           []string
	}{
		"single domain returns its groups": {
			domains:        []string{"freepik.com"},
			groupsByDomain: map[string][]string{"freepik.com": {"dev@freepik.com", "all@freepik.com"}},
			want:           []string{"dev@freepik.com", "all@freepik.com"},
		},
		"groups from every domain are merged": {
			domains: []string{"freepik.com", "magnific.com"},
			groupsByDomain: map[string][]string{
				"freepik.com":  {"dev@freepik.com"},
				"magnific.com": {"ops@magnific.com"},
			},
			want: []string{"dev@freepik.com", "ops@magnific.com"},
		},
		"a group shared across domains appears once": {
			domains: []string{"freepik.com", "magnific.com"},
			groupsByDomain: map[string][]string{
				"freepik.com":  {"shared@corp.com", "dev@freepik.com"},
				"magnific.com": {"shared@corp.com"},
			},
			want: []string{"shared@corp.com", "dev@freepik.com"},
		},
		"user with no groups anywhere yields nothing": {
			domains:        []string{"freepik.com", "magnific.com"},
			groupsByDomain: map[string][]string{},
			want:           nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &Runner{
				gsuiteDomains: tc.domains,
				gsuiteCli:     &fakeGsuiteClient{groupsByDomain: tc.groupsByDomain},
			}

			got, err := r.getGsuiteGroupsForUser("user@corp.com")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A failure on any domain must abort the union so a transient error never yields a partial
// list that would trigger spurious group removals during reconcile.
func TestGetGsuiteGroupsForUserPropagatesDomainError(t *testing.T) {
	boom := errors.New("api unavailable")
	r := &Runner{
		gsuiteDomains: []string{"freepik.com", "magnific.com"},
		gsuiteCli: &fakeGsuiteClient{
			groupsByDomain: map[string][]string{"freepik.com": {"dev@freepik.com"}},
			errByDomain:    map[string]error{"magnific.com": boom},
		},
	}

	got, err := r.getGsuiteGroupsForUser("user@corp.com")
	if err == nil {
		t.Fatalf("expected error when a domain fails, got groups %v", got)
	}
	if got != nil {
		t.Fatalf("expected no groups on error, got %v", got)
	}
}

// userDelayFromRate must convert users-per-minute into a pause and never divide by zero.
func TestUserDelayFromRate(t *testing.T) {
	tests := map[string]struct {
		rate int
		want time.Duration
	}{
		"sixty per minute is one second": {rate: 60, want: time.Second},
		"one per minute is one minute":   {rate: 1, want: time.Minute},
		"zero disables throttling":       {rate: 0, want: 0},
		"negative disables throttling":   {rate: -5, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := userDelayFromRate(tc.rate); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
