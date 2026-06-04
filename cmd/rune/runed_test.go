package main

import (
	"reflect"
	"testing"
)

func TestExtractDetachFlag(t *testing.T) {
	cases := []struct {
		name       string
		input      []string
		wantDetach bool
		wantRest   []string
	}{
		{
			name:       "absent",
			input:      []string{"--foreground", "--verbose"},
			wantDetach: false,
			wantRest:   []string{"--foreground", "--verbose"},
		},
		{
			name:       "double-dash form",
			input:      []string{"--detach"},
			wantDetach: true,
			wantRest:   []string{},
		},
		{
			name:       "single-dash form",
			input:      []string{"-detach"},
			wantDetach: true,
			wantRest:   []string{},
		},
		{
			name:       "mixed flags",
			input:      []string{"--detach", "--foreground", "--log-level=debug"},
			wantDetach: true,
			wantRest:   []string{"--foreground", "--log-level=debug"},
		},
		{
			name:       "no args",
			input:      []string{},
			wantDetach: false,
			wantRest:   []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDetach, gotRest := extractDetachFlag(tc.input)
			if gotDetach != tc.wantDetach {
				t.Errorf("detach = %v, want %v", gotDetach, tc.wantDetach)
			}
			if !reflect.DeepEqual(gotRest, tc.wantRest) {
				t.Errorf("rest = %#v, want %#v", gotRest, tc.wantRest)
			}
		})
	}
}
