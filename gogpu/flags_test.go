package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFlags(t *testing.T) {
	t.Parallel()
	usage := strings.Join([]string{
		"  -fractal string",
		`    	canonical fractal name ("seahorse", "dendrite", etc) (default "elephant")`,
		"  -theme string",
		`    	color scheme ("green" or "red") (default "green")`,
		"  -type string",
		`    	fractal type ("mandelbrot" or "julia") (default "mandelbrot")`,
	}, "\n")
	tests := []struct{
		name        string
		args        []string
		wantFractal *fractal
		wantTheme   string
		wantOut     string
		err         string // zero value means no error expected (err113)
	}{{
		name: "default mandelbrot fractal",
		args: []string{
			"-type",    "mandelbrot",
			"-fractal", "elephant",
			"-theme",   "green",
		},
		wantTheme:   "green",
		wantFractal: &fractal{
			kind:      "mandelbrot",
			name:      "elephant valley",
			titleText: "mandelbrot - elephant valley (0.2777, 0.0073i)",
			params:    &parameters{xReal: 0.2777, yImag: 0.0073},
		},
	}, {
		name:        "default julia fractal",
		args:        []string{"-type", "julia"},
		wantTheme:   "green",
		wantFractal: &fractal{
			kind:      "julia",
			name:      "spiral galaxy",
			titleText: "julia - spiral galaxy (c = -0.8 + 0.156i)",
			params:    &parameters{cReal: -0.8, cImag: 0.156},
		},
	}, {
		name:        "ignored args",
		args:        []string{"foo", "bar"},
		wantTheme:   "green",
		wantOut:     "ignored arguments: foo, bar\n",
		wantFractal: &fractal{
			kind:      "mandelbrot",
			name:      "elephant valley",
			titleText: "mandelbrot - elephant valley (0.2777, 0.0073i)",
			params:    &parameters{xReal: 0.2777, yImag: 0.0073},
		},
	}, {
		name:    "invalid -type value specified",
		args:    []string{"-type", "invalid"},
		err:     `invalid fractal type specified: "invalid"`,
		wantOut: strings.Join([]string{
			"invalid -type value specified usage:",
			"",
			usage,
			"",
			"valid values for -type:",
			"",
			"  julia",
			"  mandelbrot",
			"\n"}, "\n"),
	}, {
		name:    "invalid -fractal value specified",
		args:    []string{"-fractal", "invalid"},
		err:     `invalid fractal identifier specified: "invalid"`,
		wantOut: strings.Join([]string{
			"invalid -fractal value specified usage:",
			"",
			usage,
			"",
			"valid values for -fractal:",
			"",
			"  -0.088, 0.6555i",
			"  -0.7436447860, 0.1318252536i",
			"  -0.74364990, 0.13188204i",
			"  elephant",
			"  lightning",
			"  scepter",
			"  seahorse",
			"  spider",
			"  starburst",
			"  wolfgang",
			"\n"}, "\n"),
	}, {
		name:    "invalid -theme value specified",
		args:    []string{"-theme", "invalid"},
		err:     `invalid theme specified: "invalid"`,
		wantOut: strings.Join([]string{
			"invalid -theme value specified usage:",
			"",
			usage,
			"",
			"valid values for -theme:",
			"",
			"  green",
			"  red",
			"\n"}, "\n"),
	}, {
		name:    "-type julia specified with invalid -fractal value specified",
		args:    []string{"-type", "julia", "-fractal", "invalid"},
		err:     `invalid fractal identifier specified: "invalid"`,
		wantOut: strings.Join([]string{
			"-type julia specified with invalid -fractal value specified usage:",
			"",
			usage,
			"",
			"valid values for -fractal:",
			"",
			"  +0.37 + 0.16i",
			"  +0.40 + 0.10i",
			"  -0.40 + 0.60i",
			"  -0.50 - 0.56i",
			"  -0.5125 + 0.5213i",
			"  -0.5251993 - 0.5251993i",
			"  -0.75 + 0.25i",
			"  -1.50 + 0.00i",
			"  basilica",
			"  cantor",
			"  dendrite",
			"  floyd",
			"  golden",
			"  mantis",
			"  rabbit",
			"  siegel",
			"  spiral",
			"\n"}, "\n"),
	}, {
		name:    "invalid flag specified",
		args:    []string{"-invalid"},
		err:     "flag provided but not defined: -invalid",
		wantOut: strings.Join([]string{
			"flag provided but not defined: -invalid",
			"invalid flag specified usage:",
			"",
			usage,
			"\n"}, "\n"),
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotOut := new(bytes.Buffer)
			fs     := flag.NewFlagSet(tt.name, flag.ContinueOnError)
			fs.SetOutput(gotOut)
			var err        error
			var gotFractal *fractal
			var gotTheme   string
			gotFractal, gotTheme, err = flags(fs, tt.args)
			if tt.err != "" {
				if err == nil {
					t.Errorf("flags(%q) did not fail", tt.name)
				}
				if tt.err != err.Error() {
					t.Errorf("flags(%q) returned %q; expected %q\n", tt.name, err, tt.err)
				}
			}
			if diff := cmp.Diff(tt.wantOut, gotOut.String()); diff != "" {
				t.Errorf("flags(%q) usage message mismatch (-want +got):\n%s", tt.name, diff)
			}
			if diff := cmp.Diff(tt.wantFractal, gotFractal, cmpOpts()); diff != "" {
				t.Errorf("flags(%q) fractal mismatch (-want +got):\n%s", tt.name, diff)
			}
			if diff := cmp.Diff(tt.wantTheme, gotTheme); diff != "" {
				t.Errorf("flags(%q) theme mismatch (-want +got):\n%s", tt.name, diff)
			}
		})
	}
}

func TestFilterArgs(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name string
		args []string
		want []string
	}{{
		name: "no extra args",
		args: []string{"-type", "foo", "-fractal", "bar", "-theme", "baz"},
		want: []string{"-type", "foo", "-fractal", "bar", "-theme", "baz"},
	}, {
		name: "extra args",
		args: []string{"-type", "foo", "-fractal", "bar", "-theme", "baz", "--", "boo", "hoo"},
		want: []string{"boo", "hoo"},
	}, {
		name: "invalid args",
		args: []string{"foo", "bar", "--", "baz", "boo"},
		want: []string{"baz", "boo"},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterArgs(tt.args)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("filterArgs(%v) mismatch (-want +got):\n%s", tt.args, diff)
			}
		})
	}
}
