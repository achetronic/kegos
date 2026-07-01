// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// resolveInt must prefer an explicit flag, then a parseable env var, then the default.
func TestResolveInt(t *testing.T) {
	tests := map[string]struct {
		flagSet   bool
		flagValue int
		envRaw    string
		want      int
	}{
		"env value is honoured when flag not set": {flagSet: false, flagValue: 60, envRaw: "120", want: 120},
		"explicit flag beats env":                 {flagSet: true, flagValue: 30, envRaw: "120", want: 30},
		"empty env falls back to default":         {flagSet: false, flagValue: 60, envRaw: "", want: 60},
		"garbage env falls back to default":       {flagSet: false, flagValue: 60, envRaw: "fast", want: 60},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resolveInt(tc.flagSet, tc.flagValue, tc.envRaw); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
