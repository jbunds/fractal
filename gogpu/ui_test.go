package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestIsUnnamed(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name       string
		identifier string
		want       bool
	}{{
		name:       "named",
		identifier: "named",
	}, {
		name:       "unnamed",
		identifier: "-this is unnamed",
		want:       true,
	}, {
		name:       "also unnamed",
		identifier: "+this is also unnamed",
		want:       true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUnnamed(tt.identifier)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("isUnnamed(%q) mismatch (-want +got):\n%s", tt.identifier, diff)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name       string
		identifier string
		want       string
	}{{
		name:       "named",
		identifier: "named",
		want:       "named",
	}, {
		name:       "unnamed",
		identifier: "-unnamed",
		want:       "unnamed",
	}, {
		name:       "also unnamed",
		identifier: "+unnamed",
		want:       "unnamed",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeName(tt.identifier)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("normalizeName(%q) mismatch (-want +got):\n%s", tt.identifier, diff)
			}
		})
	}
}
