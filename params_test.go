package main

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFractals(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name string
		want map[string]*fractal
	}{{
		name: "all fractals",
		want: map[string]*fractal{
			"+0.37 + 0.16i": {
				kind:      "julia",
				name:      "+0.37 + 0.16i",
				titleText: "julia - unnamed (c = 0.37 + 0.16i)",
				params:    &parameters{cReal: 0.37, cImag: 0.16, powScale: 1},
			},
			"+0.40 + 0.10i": {
				kind:      "julia",
				name:      "+0.40 + 0.10i",
				titleText: "julia - unnamed (c = 0.4 + 0.1i)",
				params:    &parameters{
					xReal:    0.3769602426587254,
					yImag:    0.40637271301921807,
					cReal:    0.4,
					cImag:    0.1,
					powScale: 1,
				},
			},
			"-0.088, 0.6555i": {
				kind:      "mandelbrot",
				name:      "-0.088, 0.6555i",
				titleText: "mandelbrot - unnamed (-0.088, 0.6555i)",
				params:    &parameters{xReal: -0.088, yImag: 0.6555},
			},
			"-0.40 + 0.60i": {
				kind:      "julia",
				name:      "-0.40 + 0.60i",
				titleText: "julia - unnamed (c = -0.4 + 0.6i)",
				params:    &parameters{
					xReal: -0.37595385170901174,
					yImag:  0.34248379570988946,
					cReal: -0.4,
					cImag:  0.6,
				},
			},
			"-0.50 - 0.56i": {
				kind:      "julia",
				name:      "-0.50 - 0.56i",
				titleText: "julia - unnamed (c = -0.5 + -0.56i)",
				params:    &parameters{
					xReal:   -0.4181508962991045,
					yImag:   -0.30496076530407795,
					cReal:   -0.5,
					cImag:   -0.56,
					powScale: 1,
				},
			},
			"-0.5125 + 0.5213i": {
				kind:      "julia",
				name:      "-0.5125 + 0.5213i",
				titleText: "julia - unnamed (c = -0.5125 + 0.5213i)",
				params:    &parameters{cReal: -0.5125, cImag: 0.5213},
			},
			"-0.5251993 - 0.5251993i": {
				kind:      "julia",
				name:      "-0.5251993 - 0.5251993i",
				titleText: "julia - unnamed (c = -0.5251993 + -0.5251993i)",
				params:    &parameters{
					xReal: -0.42508333248568575,
					yImag: -0.2838659402655094,
					cReal: -0.5251993,
					cImag: -0.5251993,
				},
			},
			"-0.7436447860, 0.1318252536i": {
				kind:      "mandelbrot",
				name:      "-0.7436447860, 0.1318252536i",
				titleText: "mandelbrot - unnamed (-0.743644786, 0.1318252536i)",
				params:    &parameters{xReal: -0.743644786, yImag: 0.1318252536},
			},
			"-0.74364990, 0.13188204i": {
				kind:      "mandelbrot",
				name:      "-0.74364990, 0.13188204i",
				titleText: "mandelbrot - unnamed (-0.7436499, 0.13188204i)",
				params:    &parameters{xReal: -0.7436499, yImag: 0.13188204},
			},
			"-0.75 + 0.25i": {
				kind:      "julia",
				name:      "-0.75 + 0.25i",
				titleText: "julia - unnamed (c = -0.75 + 0.25i)",
				params:    &parameters{
					xReal: -0.5076647275766915,
					yImag:  0.12404919670117806,
					cReal: -0.75,
					cImag:  0.25,
				},
			},
			"-1.50 + 0.00i": {
				kind:      "julia",
				name:      "-1.50 + 0.00i",
				titleText: "julia - unnamed (c = -1.5 + 0i)",
				params:    &parameters{cReal: -1.5, powScale: 1},
			},
			"basilica": {
				kind:      "julia",
				name:      "basilica",
				titleText: "julia - basilica (c = -0.75 + 0i)",
				params:    &parameters{xReal: -0.5, cReal: -0.75, powScale: 1},
			},
			"cantor": {
				kind:      "julia",
				name:      "cantor dust",
				titleText: "julia - cantor dust (c = 1 + 0i)",
				params:    &parameters{
					xReal:    0.5,
					yImag:   -0.8660254037844386,
					cReal:    1,
					powScale: 1,
				},
			},
			"dendrite": {
				kind:      "julia",
				name:      "dendrite",
				titleText: "julia - dendrite (c = 0 + 1i)",
				params:    &parameters{
					xReal:   -0.30024259022012045,
					yImag:    0.6248105338438266,
					cImag:    1,
					powScale: 1,
				},
			},
			"elephant": {
				kind:      "mandelbrot",
				name:      "elephant valley",
				titleText: "mandelbrot - elephant valley (0.2777, 0.0073i)",
				params:    &parameters{xReal: 0.2777, yImag: 0.0073},
			},
			"floyd": {
				kind:      "julia",
				name:      "pink floyd",
				titleText: "julia - pink floyd (c = 0.285 + 0.01i)",
				params:    &parameters{
					xReal: 0.47353729561814784,
					yImag: 0.18894516327019664,
					cReal: 0.285,
					cImag: 0.01,
				},
			},
			"golden": {
				kind:      "julia",
				name:      "golden",
				titleText: "julia - golden (c = (φ - 2) + (φ - 1)i)",
				params:    &parameters{
					xReal: -0.3706044986538717,
					yImag:  0.35494532230507586,
					cReal:  math.Phi - 2.0,
					cImag:  math.Phi - 1.0,
				},
			},
			"lightning": {
				kind:      "mandelbrot",
				name:      "lightning branch",
				titleText: "mandelbrot - lightning branch (-1.250662834, 0.020126938i)",
				params:    &parameters{xReal: -1.250662834, yImag: 0.020126938},
			},
			"mantis": {
				kind:      "julia",
				name:      "praying mantis",
				titleText: "julia - praying mantis (c = 0.285 + 0.013i)",
				params:    &parameters{
					xReal: 0.46582172177252823,
					yImag: 0.19017926990761755,
					cReal: 0.285,
					cImag: 0.013,
				},
			},
			"rabbit": {
				kind:      "julia",
				name:      "rabbit",
				titleText: "julia - rabbit (c = -0.122561 + 0.744862i)",
				params:    &parameters{
					xReal:   -0.2763376130307137,
					yImag:    0.47972814114478024,
					cReal:   -0.122561,
					cImag:    0.744862,
					powScale: 1,
				},
			},
			"scepter": {
				kind:      "mandelbrot",
				name:      "scepter",
				titleText: "mandelbrot - scepter (-1.45, 0i)",
				params:    &parameters{xReal: -1.45},
			},
			"seahorse": {
				kind:      "mandelbrot",
				name:      "seahorse valley",
				titleText: "mandelbrot - seahorse valley (-0.743643887037151, 0.13182590420533i)",
				params:    &parameters{xReal: -0.743643887037151, yImag: 0.13182590420533},
			},
			"siegel": {
				kind:      "julia",
				name:      "siegel disc",
				titleText: "julia - siegel disc (c = -0.390541 + -0.586788i)",
				params:    &parameters{cReal: -0.390541, cImag: -0.586788, powScale: 1},
			},
			"spider": {
				kind:      "mandelbrot",
				name:      "spider",
				titleText: "mandelbrot - spider (-1.4063, 0i)",
				params:    &parameters{xReal: -1.4063},
			},
			"spiral": {
				kind:      "julia",
				name:      "spiral galaxy",
				titleText: "julia - spiral galaxy (c = -0.8 + 0.156i)",
				params:    &parameters{cReal: -0.8, cImag: 0.156},
			},
			"starburst": {
				kind:      "mandelbrot",
				name:      "starburst",
				titleText: "mandelbrot - starburst (-0.77568377, 0.13646737i)",
				params:    &parameters{xReal: -0.77568377, yImag: 0.13646737},
			},
			"wolfgang": {
				kind:      "mandelbrot",
				name:      "wolfgang beyer",
				titleText: "mandelbrot - wolfgang beyer (-0.7436438885706, 0.1318259043124i)",
				params:    &parameters{xReal: -0.7436438885706, yImag: 0.1318259043124},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, fractals(), cmpOpts()); diff != "" {
				t.Errorf("fractals() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetJuliaFractalZoomPoints(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name     string
		fractals map[string]*fractal
		want     map[string]*fractal
	}{{
		name:    "calculated zoom point is α fixed point",
		fractals: map[string]*fractal{
			"basilica": { // zoom point (xReal, yImag): 0, 0 => α fixed point
				kind:   "julia",
				name:   "basilica",
				params: &parameters{cReal: -0.75, powScale: 1},
			},
			"golden": { // zoom point (xReal, yImag): 0, 0 => α fixed point
				kind:   "julia",
				name:   "golden",
				params: &parameters{cReal: math.Phi - 2.0, cImag: math.Phi - 1.0},
			},
			"rabbit": { // zoom point (xReal, yImag): 0, 0 => α fixed point
				kind:      "julia",
				name:      "rabbit",
				titleText: "julia - rabbit (c = -0.122561 + 0.744862i)",
				params: &parameters{cReal: -0.122561, cImag: 0.744862, powScale: 1},
			},
		},
		want: map[string]*fractal{
			"basilica": {
				kind:   "julia",
				name:   "basilica",
				params: &parameters{
					xReal:   -0.5, // zoom point == α fixed point
					yImag:    0.0, // zoom point == α fixed point
					cReal:   -0.75,
					powScale: 1,
				},
			},
			"golden": {
				kind: "julia",
				name: "golden",
				params: &parameters{
					xReal: -0.3706044986538717,  // zoom point == α fixed point
					yImag:  0.35494532230507586, // zoom point == α fixed point
					cReal: math.Phi - 2.0,
					cImag: math.Phi - 1.0,
				},
			},
			"rabbit": {
				kind:      "julia",
				name:      "rabbit",
				titleText: "julia - rabbit (c = -0.122561 + 0.744862i)",
				params: &parameters{
					xReal:   -0.2763376130307137,  // zoom point == α fixed point
					yImag:    0.47972814114478024, // zoom point == α fixed point
					cReal:   -0.122561,
					cImag:    0.744862,
					powScale: 1,
				},
			},
		},
	}, {
		name:    "calculated zoom point is critical point (origin)",
		fractals: map[string]*fractal{
			"siegel": { // zoom point == critical point (origin)
				kind:   "julia",
				name:   "siegel disc",
				params: &parameters{cReal: -0.390541, cImag: -0.586788, powScale: 1},
			},
			"spiral": { // zoom point == critical point (origin)
				kind:   "julia",
				name:   "spiral galaxy",
				params: &parameters{cReal: -0.8, cImag: 0.156},
			},
		},
		want: map[string]*fractal{
			"siegel": {
				kind:   "julia",
				name:   "siegel disc",
				params: &parameters{
					xReal:    0.0, // zoom point == critical point (origin)
					yImag:    0.0, // zoom point == critical point (origin)
					cReal:   -0.390541,
					cImag:   -0.586788,
					powScale: 1,
				},
			},
			"spiral": {
				kind:   "julia",
				name:   "spiral galaxy",
				params: &parameters{
					xReal:  0.0, // zoom point == critical point (origin)
					yImag:  0.0, // zoom point == critical point (origin)
					cReal: -0.8,
					cImag:  0.156,
				},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			setJuliaFractalZoomPoints(tt.fractals)
			if diff := cmp.Diff(tt.want, tt.fractals, cmpOpts()); diff != "" {
				t.Errorf("setJuliaFractalZoomPoints() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetTitles(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name     string
		fractals map[string]*fractal
		want     map[string]*fractal
	}{{
		name:     "elephant",
		fractals: map[string]*fractal{
			"elephant": {
				kind:   "mandelbrot",
				name:   "elephant valley",
				params: &parameters{xReal: 0.2777, yImag: 0.0073},
			},
		},
		want: map[string]*fractal{
			"elephant": {
				kind:      "mandelbrot",
				name:      "elephant valley",
				titleText: "mandelbrot - elephant valley (0.2777, 0.0073i)",
				params:    &parameters{xReal: 0.2777, yImag: 0.0073},
			},
		},
	}, {
		name:     "seahorse",
		fractals: map[string]*fractal{
			"seahorse": {
				kind:      "mandelbrot",
				name:      "seahorse valley",
				params:    &parameters{xReal: -0.743643887037151, yImag: 0.13182590420533},
			},
		},
		want: map[string]*fractal{
			"seahorse": {
				kind:      "mandelbrot",
				name:      "seahorse valley",
				titleText: "mandelbrot - seahorse valley (-0.743643887037151, 0.13182590420533i)",
				params:    &parameters{xReal: -0.743643887037151, yImag: 0.13182590420533},
			},
		},
	}, {
		name: "dendrite",
		fractals: map[string]*fractal{
			"dendrite": {
				kind:      "julia",
				name:      "dendrite",
				params: &parameters{
					xReal:    -0.30024259022012045,
					yImag:     0.6248105338438266,
					cImag:     1,
					powScale:  1,
				},
			},
		},
		want: map[string]*fractal{
			"dendrite": {
				kind:      "julia",
				name:      "dendrite",
				titleText: "julia - dendrite (c = 0 + 1i)",
				params:    &parameters{
					xReal:    -0.30024259022012045,
					yImag:     0.6248105338438266,
					cImag:     1,
					powScale:  1,
				},
			},
		},
	}, {
		name: "golden",
		fractals: map[string]*fractal{
			"golden": {
				kind:   "julia",
				name:   "golden",
				params: &parameters{
					xReal: -0.3706044986538717,
					yImag:  0.35494532230507586,
					cReal:  math.Phi - 2.0,
					cImag:  math.Phi - 1.0,
				},
			},
		},
		want: map[string]*fractal{
			"golden": {
				kind:      "julia",
				name:      "golden",
				titleText: "julia - golden (c = (φ - 2) + (φ - 1)i)",
				params: &parameters{
					xReal: -0.3706044986538717,
					yImag:  0.35494532230507586,
					cReal:  math.Phi - 2.0,
					cImag:  math.Phi - 1.0,
				},
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			setTitles(tt.fractals)
			if diff := cmp.Diff(tt.want, tt.fractals, cmpOpts()); diff != "" {
				t.Errorf("setTitles() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSetMaxIter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fractals   map[string]*fractal
		frameCount int
		want       float64
	}{{
		name:       "mandelbrot",
		fractals:   map[string]*fractal{"mandelbrot": {kind: "mandelbrot", params: &parameters{}}},
		frameCount: 100,
		want:       530,
	}, {
		name:      "julia",
		fractals:   map[string]*fractal{"julia": {kind: "julia", params: &parameters{}}},
		frameCount: 200,
		want:       620,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			setMaxIter(tt.fractals)
			got := tt.fractals[tt.name].params.maxIter(tt.frameCount)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("setMaxIter() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
