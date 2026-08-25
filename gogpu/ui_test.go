package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestLabels(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name          string
		fractals      map[string]*fractal
		wantEntries   map[string]map[string]string
		wantMenuItems map[string][]string
	}{
		{
			name:     "succeeds",
			fractals: map[string]*fractal{
				"foo": &fractal{
					kind:   "mandelbrot",
					name:   "foo",
					params: &parameters{xReal: -2, yImag: 1},
				},
				"bar": &fractal{
					kind:   "mandelbrot",
					name:   "bar",
					params: &parameters{xReal: -1, yImag: 2},
				},
				"+1, -1i": &fractal{
					kind:   "mandelbrot",
					name:   "+1, -1",
					params: &parameters{xReal: 1, yImag: -1},
				},
				"golden": &fractal{
					kind:   "julia",
					name:   "golden",
					params: &parameters{cReal: 2, cImag: -2},
				},
				"-1 + 1i": &fractal{
					kind:   "julia",
					name:   "-1 + 1i",
					params: &parameters{cReal: -1, cImag: 1},
				},
			},
			wantEntries:   map[string]map[string]string{
				"mandelbrot": {
					"bar":     "bar:      -1, 2i",
					"foo":     "foo:      -2, 1i",
					"+1, -1i": "unnamed:   1, -1i",
				},
				"julia":      {
					"golden":  "golden:   c = (φ - 2) + (φ - 1)i",
					"-1 + 1i": "unnamed:  c = -1 + 1i",
				},
			},
			wantMenuItems: map[string][]string{
				"mandelbrot": []string{
					"bar",
					"foo",
					"+1, -1i",
				},
				"julia":      []string{
					"golden",
					"-1 + 1i",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotEntries, gotMenuItems := labels(tt.fractals)
			if diff := cmp.Diff(tt.wantEntries, gotEntries); diff != "" {
				t.Errorf("labels(%q) mismatch (-want +got):\n%s", tt.wantEntries, diff)
			}
			if diff := cmp.Diff(tt.wantMenuItems, gotMenuItems); diff != "" {
				t.Errorf("labels(%q) mismatch (-want +got):\n%s", tt.wantMenuItems, diff)
			}
		})
	}
}

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
