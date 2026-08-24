package main

import (
	"fmt"
	"iter"
	"maps"
	"math"
	"math/cmplx"
	"slices"
	"strings"
)

// parameters represents a named (or unnamed) preset set of parameters:
//   - the real and imaginary (x, y) coordinates of a target point of interest in the Mandelbrot set; or
//   - the real and imaginary components of the complex term 𝑐 in 𝑓(𝑧) = 𝑧² + 𝑐 which defines the filled Julia set
type parameters struct {
	xReal, yImag float64
	cReal, cImag float64
	powScale     uint32            // used in the shader code for Julia fractals to adjust gradient steepness
	maxIter      func(int) float64 // used in the shader code to set the maximum number of iterations used to compute interior boundaries
}

type fractal struct {
	kind,
	name,
	titleText string
	params    *parameters
}

func fractals() map[string]*fractal {
	fractals := map[string]*fractal{
		// default, because it looks nice when magnified
		"elephant": {
			kind:  "mandelbrot",
			name:  "elephant valley",
			params: &parameters{xReal: 0.2777, yImag: 0.0073}},
		"lightning": {
			kind:  "mandelbrot",
			name:  "lightning branch",
			params: &parameters{xReal: -1.250662834, yImag: 0.020126938}},
		// adjusted seahorse valley coordinates suitable for deep zoom
		"seahorse": {
			kind:  "mandelbrot",
			name:  "seahorse valley",
			params: &parameters{xReal: -0.743643887037151, yImag: 0.131825904205330}},
		"scepter": {
			kind:  "mandelbrot",
			name:  "scepter",
			params: &parameters{xReal: -1.45, yImag: 0.0}},
		"spider": {
			kind:   "mandelbrot",
			name:   "spider",
			params: &parameters{xReal: -1.4063, yImag: 0.0}},
		"starburst": {
			kind:  "mandelbrot",
			name:  "starburst",
			params: &parameters{xReal: -0.77568377, yImag: 0.13646737}},
		"wolfgang": {
			kind:   "mandelbrot",
			name:   "wolfgang beyer",
			params: &parameters{xReal: -0.7436438885706, yImag: 0.1318259043124}},
		"-0.088, 0.6555": {
			kind:   "mandelbrot",
			name:   "-0.088, 0.6555",
			params: &parameters{xReal: -0.088, yImag: 0.6555}},
		// high-quality deep-zoom images uploaded by https://commons.wikimedia.org/wiki/User:Wolfgangbeyer
		// https://en.wikipedia.org/wiki/File:Mandel_zoom_05_tail_part.jpg
		"-0.74364990, 0.13188204i": {
			kind:   "mandelbrot",
			name:   "-0.74364990, 0.13188204i",
			params: &parameters{xReal: -0.74364990, yImag: 0.13188204}},
		// https://en.wikipedia.org/wiki/File:Mandel_zoom_08_satellite_antenna.jpg
		"-0.7436447860, 0.1318252536i": {
			kind:   "mandelbrot",
			name:   "-0.7436447860, 0.1318252536i",
			params: &parameters{xReal: -0.7436447860, yImag: 0.1318252536}},
		// default, because it's well-known and looks nice
		"spiral": {
			kind:   "julia",
			name:   "spiral galaxy",
			params: &parameters{cReal: -0.8, cImag: 0.156}},
		"basilica": {
			kind:   "julia",
			name:   "basilica",
			params: &parameters{cReal: -0.75, cImag: 0.0, powScale: 1}},
		// TODO(jbunds): improve color gradients for the cantor dust fractal
		"cantor": {
			kind:   "julia",
			name:   "cantor dust",
			params: &parameters{cReal: 1.0, cImag: 0.0, powScale: 1}},
		"dendrite": {
			kind:   "julia",
			name:   "dendrite",
			params: &parameters{cReal: 0.0, cImag: 1.0, powScale: 1}},
		// https://commons.wikimedia.org/wiki/File:Julia_set_(highres_01).jpg
		"floyd": {
			kind:   "julia",
			name:   "pink floyd",
			params: &parameters{cReal: 0.285, cImag: 0.01}},
		"golden": {
			kind:   "julia",
			name:   "golden",
			params: &parameters{cReal: math.Phi - 2.0, cImag: math.Phi - 1.0}},
		// https://commons.wikimedia.org/wiki/File:Julia_set_(highres_02).jpg
		"mantis": {
			kind:   "julia",
			name:   "praying mantis",
			params: &parameters{cReal: 0.285, cImag: 0.013}},
		"rabbit": {
			kind:   "julia",
			name:   "rabbit",
			params: &parameters{cReal: -0.122561, cImag: 0.744862, powScale: 1}},
		"siegel": {
			kind:   "julia",
			name:   "siegel disc",
			params: &parameters{cReal: -0.390541, cImag: -0.586788, powScale: 1}},
		// most of these were taken from
		// https://e.math.cornell.edu/people/belk/dynamicalsystems/NotesJuliaMandelbrot.pdf
		// and i'm aware of no associated canonical names for any of these complex constants
		"+0.37 + 0.16i": {
			kind:   "julia",
			name:   "+0.37 + 0.16i",
			params: &parameters{cReal: 0.37, cImag:  0.16, powScale: 1}},
		"+0.40 + 0.10i": {
			kind:   "julia",
			name:   "+0.40 + 0.10i",
			params: &parameters{cReal: 0.40, cImag: 0.10, powScale: 1}},
		"-0.40 + 0.60i": {
			kind:   "julia",
			name:   "-0.40 + 0.60i",
			params: &parameters{cReal: -0.40, cImag:  0.60}},
		"-0.50 - 0.56i": {
			kind:   "julia",
			name:   "-0.50 - 0.56i",
			params: &parameters{cReal: -0.50, cImag: -0.56, powScale: 1}},
		// TODO(jbunds): improve color gradients for the following fractal
		"-0.75 + 0.25i": {
			kind:   "julia",
			name:   "-0.75 + 0.25i",
			params: &parameters{cReal: -0.75, cImag:  0.25}},
		"-1.50 + 0.00i": {
			kind:   "julia",
			name:   "-1.50 + 0.00i",
			params: &parameters{cReal: -1.50, cImag:  0.00, powScale: 1}},
		// https://en.wikipedia.org/wiki/Julia_set#Quadratic_polynomials
		"-0.5125 + 0.5213i": {
			kind:   "julia",
			name:   "-0.5125 + 0.5213i",
			params: &parameters{cReal: -0.5125, cImag: 0.5213}},
		"-0.5251993 - 0.5251993i": {
			kind:   "julia",
			name:   "-0.5251993 - 0.5251993i",
			params: &parameters{cReal: -0.5251993, cImag: -0.5251993}},
	}

	setJuliaFractalZoomPoints(fractals)
	setTitles(fractals)
	setMaxIter(fractals)

	return fractals
}

// setJuliaFractalZoomPoints sets interesting target coordinates to zoom in on.
func setJuliaFractalZoomPoints(fractals map[string]*fractal) {
	for _, fractal := range fractals { // locate interesting target coordinates to zoom in on, with some exceptions
		if fractal.kind == "mandelbrot" { continue }
		switch fractal.name {
		case "siegel disc",
		     "spiral galaxy",
		     "+0.37 + 0.16i",
		     "-1.50 + 0.00i",
		     "-0.5125 + 0.5213i": // zoom in on the critical point (origin) rather than the α fixed point for these fractals
			fractal.params.xReal,
			fractal.params.yImag = 0, 0
		default:
			c := complex(fractal.params.cReal, fractal.params.cImag)
			z := (1.0 - cmplx.Sqrt(1.0 - 4.0 * c)) / 2.0 // calculate the α fixed point by solving 𝑧² - 𝑧 + 𝑐 = 0
			fractal.params.xReal,
			fractal.params.yImag = real(z), imag(z)
		}
	}
}

// setTitles sets the title text displayed in the primary window.
func setTitles(fractals map[string]*fractal) {
	for _, fractal := range fractals {
		displayName := fractal.name
		if strings.ContainsAny(string(displayName[0]), "+-0") { // hack to avoid using regexp
			displayName = "unnamed"
		}
		switch fractal.kind {
		case "mandelbrot":
			fractal.titleText = fmt.Sprintf("%s - %s (%v, %vi)", fractal.kind, displayName, fractal.params.xReal, fractal.params.yImag)
		case "julia":
			displayCReal := fmt.Sprintf("%v", fractal.params.cReal) // hack to avoid using strconv
			displayCImag := fmt.Sprintf("%v", fractal.params.cImag)
			if fractal.name == "golden" { // special handle "golden" since φ is irrational, hence its decimal representation is long
				displayCReal = "(φ - 2)"
				displayCImag = "(φ - 1)"
			}
			fractal.titleText = fmt.Sprintf("%s - %s (c = %s + %si)", fractal.kind, displayName, displayCReal, displayCImag)
		}
	}
}

// setMaxIter sets the frame count-dependent function used to calculate the maximum number of iterations used to compute interior bounaries.
func setMaxIter(fractals map[string]*fractal) {
	for _, fractal := range fractals {
		switch fractal.kind {
		case "mandelbrot":
			fractal.params.maxIter = func(frameCount int) float64 {
				return float64(baseIterations) + float64(frameCount) * linearGrowthFactor
			}
		case "julia":
			fractal.params.maxIter = func(frameCount int) float64 {
				return float64(baseIterations) + (float64(frameCount * frameCount) * quadraticGrowthFactor)
			}
		}
	}
}

// kinds returns a slice of all fractal kinds.
func kinds(fractals map[string]*fractal) []string {
	kinds := make(map[string]struct{})
	for fractal := range maps.Values(fractals) {
		kinds[fractal.kind] = struct{}{}
	}
	return slices.Sorted(maps.Keys(kinds))
}

// fractalNamesByKind returns an iterator over all fractal names of the specified kind.
func fractalNamesByKind(fractals map[string]*fractal, kind string) iter.Seq[string] { // TODO(jbunds): clean up this and uiLabels
	return func(yield func(string) bool) {
		for name, fractal := range fractals {
			if fractal.kind == kind {
				if !yield(name) {
					return
				}
			}
		}
	}
}
