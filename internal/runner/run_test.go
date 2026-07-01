// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package runner

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// fakeGsuiteClient records the domain and user key it was queried with and returns canned data.
type fakeGsuiteClient struct {
	groupsByDomain map[string][]string
	primaryEmail   string
	primaryErr     error

	gotDomain  string
	gotUserKey string
}

func (f *fakeGsuiteClient) GetGroupsFromUser(domain string, user string) ([]string, error) {
	f.gotDomain = domain
	f.gotUserKey = user
	return f.groupsByDomain[domain], nil
}

func (f *fakeGsuiteClient) GetUserPrimaryEmail(userKey string) (string, error) {
	if f.primaryErr != nil {
		return "", f.primaryErr
	}
	return f.primaryEmail, nil
}

// domainFromEmail must extract the domain and reject malformed addresses.
func TestDomainFromEmail(t *testing.T) {
	tests := map[string]struct {
		email     string
		want      string
		wantError bool
	}{
		"plain address":        {email: "ahernandez@freepik.com", want: "freepik.com"},
		"subdomain":            {email: "user@sales.freepik.com", want: "sales.freepik.com"},
		"no at sign":           {email: "notanemail", wantError: true},
		"empty local part":     {email: "@freepik.com", wantError: true},
		"empty domain part":    {email: "user@", wantError: true},
		"empty string":         {email: "", wantError: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := domainFromEmail(tc.email)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.email, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Without alias resolution the domain is derived from the Keycloak username itself.
func TestGetGsuiteGroupsForUserDerivesDomainFromUsername(t *testing.T) {
	fake := &fakeGsuiteClient{
		groupsByDomain: map[string][]string{"freepik.com": {"dev@freepik.com", "all@freepik.com"}},
	}
	r := &Runner{resolveAliases: false, gsuiteCli: fake}

	got, err := r.getGsuiteGroupsForUser("ahernandez@freepik.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"dev@freepik.com", "all@freepik.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if fake.gotDomain != "freepik.com" {
		t.Fatalf("queried domain %q, want freepik.com", fake.gotDomain)
	}
	if fake.gotUserKey != "ahernandez@freepik.com" {
		t.Fatalf("queried userKey %q, want the raw username", fake.gotUserKey)
	}
}

// With alias resolution enabled the primary email drives both the domain and the userKey,
// so a user logged in through an alias domain matches its real domain.
func TestGetGsuiteGroupsForUserResolvesAlias(t *testing.T) {
	fake := &fakeGsuiteClient{
		primaryEmail:   "ahernandez@freepik.com",
		groupsByDomain: map[string][]string{"freepik.com": {"dev@freepik.com"}},
	}
	r := &Runner{resolveAliases: true, gsuiteCli: fake}

	got, err := r.getGsuiteGroupsForUser("ahernandez@magnific.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"dev@freepik.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if fake.gotDomain != "freepik.com" {
		t.Fatalf("queried domain %q, want the resolved domain freepik.com", fake.gotDomain)
	}
	if fake.gotUserKey != "ahernandez@freepik.com" {
		t.Fatalf("queried userKey %q, want the resolved primary email", fake.gotUserKey)
	}
}

// A failure resolving the primary email must abort the user without querying groups.
func TestGetGsuiteGroupsForUserPropagatesResolveError(t *testing.T) {
	fake := &fakeGsuiteClient{primaryErr: errors.New("user not found")}
	r := &Runner{resolveAliases: true, gsuiteCli: fake}

	if _, err := r.getGsuiteGroupsForUser("ghost@magnific.com"); err == nil {
		t.Fatal("expected error when primary email resolution fails")
	}
	if fake.gotDomain != "" {
		t.Fatalf("groups should not be queried on resolve failure, got domain %q", fake.gotDomain)
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
