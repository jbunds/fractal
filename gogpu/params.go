package main

import (
	"fmt"
	"math"
	"math/cmplx"
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
	params    parameters
}

// TODO(jbunds): remove the top-level kind keys and just return map[string]fractal, since
//               it's just redundant nesting which unnecessarily complicates lots of downstream code
func fractals() map[string]map[string]*fractal {
	fractals := map[string]map[string]*fractal{
		"mandelbrot": {
// TODO(jbunds): name this (or not)
"dunno": &fractal{ kind: "mandelbrot", name: "dunno", params: parameters{xReal: -.7436438885706, yImag: 0.1318259043124}},
			// default, because it looks nice when magnified
			"elephant":                     &fractal{ kind: "mandelbrot", name: "elephant valley",              params: parameters{xReal:  0.2777,            yImag: 0.0073           }},
			"lightning":                    &fractal{ kind: "mandelbrot", name: "lightning branch",             params: parameters{xReal: -1.250662834,       yImag: 0.020126938      }},
			// adjusted seahorse valley coordinates suitable for deep zoom
			"seahorse":                     &fractal{ kind: "mandelbrot", name: "seahorse valley",              params: parameters{xReal: -0.743643887037151, yImag: 0.131825904205330}},
			"scepter":                      &fractal{ kind: "mandelbrot", name: "scepter",                      params: parameters{xReal: -1.45,              yImag: 0.0              }},
			"spider":                       &fractal{ kind: "mandelbrot", name: "spider",                       params: parameters{xReal: -1.4063,            yImag: 0.0              }},
			"spiral":                       &fractal{ kind: "mandelbrot", name: "spiral",                       params: parameters{xReal: -0.088,             yImag: 0.6555           }},
			"starburst":                    &fractal{ kind: "mandelbrot", name: "starburst",                    params: parameters{xReal: -0.77568377,        yImag: 0.13646737       }},
			// high-quality deep-zoom images uploaded by https://commons.wikimedia.org/wiki/User:Wolfgangbeyer
			// https://en.wikipedia.org/wiki/File:Mandel_zoom_05_tail_part.jpg
			"-0.74364990, 0.13188204i":     &fractal{ kind: "mandelbrot", name: "-0.74364990, 0.13188204i",     params: parameters{xReal: -0.74364990,        yImag: 0.13188204       }},
			// https://en.wikipedia.org/wiki/File:Mandel_zoom_08_satellite_antenna.jpg
			"-0.7436447860, 0.1318252536i": &fractal{ kind: "mandelbrot", name: "-0.7436447860, 0.1318252536i", params: parameters{xReal: -0.7436447860,      yImag: 0.1318252536     }},
		},
		"julia": {
			// default, because it's well-known and looks nice
			"spiral":                  &fractal{ kind: "julia", name: "spiral galaxy",           params: parameters{cReal: -0.8,           cImag:  0.156                   }},
			"basilica":                &fractal{ kind: "julia", name: "basilica",                params: parameters{cReal: -0.75,          cImag:  0.0,         powScale: 1}},
			// TODO(jbunds): improve color gradients for the cantor dust fractal
			"cantor":                  &fractal{ kind: "julia", name: "cantor dust",             params: parameters{cReal:  1.0,           cImag:  0.0,         powScale: 1}},
			"dendrite":                &fractal{ kind: "julia", name: "dendrite",                params: parameters{cReal:  0.0,           cImag:  1.0,         powScale: 1}},
			// https://commons.wikimedia.org/wiki/File:Julia_set_(highres_01).jpg
			"floyd":                   &fractal{ kind: "julia", name: "pink floyd",              params: parameters{cReal:  0.285,         cImag:  0.01                    }},
			"golden":                  &fractal{ kind: "julia", name: "golden",                  params: parameters{cReal: math.Phi - 2.0, cImag: math.Phi - 1.0           }},
			// https://commons.wikimedia.org/wiki/File:Julia_set_(highres_02).jpg
			"mantis":                  &fractal{ kind: "julia", name: "praying mantis",          params: parameters{cReal:  0.285,         cImag:  0.013                   }},
			"rabbit":                  &fractal{ kind: "julia", name: "rabbit",                  params: parameters{cReal: -0.122561,      cImag:  0.744862,    powScale: 1}},
			"siegel":                  &fractal{ kind: "julia", name: "siegel disc",             params: parameters{cReal: -0.390541,      cImag: -0.586788,    powScale: 1}},
			// most of these were taken from
			// https://e.math.cornell.edu/people/belk/dynamicalsystems/NotesJuliaMandelbrot.pdf
			// and i'm aware of no associated canonical names for any of these complex constants
			"+0.37 + 0.16i":           &fractal{ kind: "julia", name: "+0.37 + 0.16i",           params: parameters{cReal:  0.37,          cImag:  0.16,        powScale: 1}},
			"+0.40 + 0.10i":           &fractal{ kind: "julia", name: "+0.40 + 0.10i",           params: parameters{cReal:  0.40,          cImag:  0.10,        powScale: 1}},
			"-0.40 + 0.60i":           &fractal{ kind: "julia", name: "-0.40 + 0.60i",           params: parameters{cReal: -0.40,          cImag:  0.60                    }},
			"-0.50 - 0.56i":           &fractal{ kind: "julia", name: "-0.50 - 0.56i",           params: parameters{cReal: -0.50,          cImag: -0.56,        powScale: 1}},
			// TODO(jbunds): improve color gradients for the following fractal
			"-0.75 + 0.25i":           &fractal{ kind: "julia", name: "-0.75 + 0.25i",           params: parameters{cReal: -0.75,          cImag:  0.25                    }},
			"-1.50 + 0.00i":           &fractal{ kind: "julia", name: "-1.50 + 0.00i",           params: parameters{cReal: -1.50,          cImag:  0.00,        powScale: 1}},
			// https://en.wikipedia.org/wiki/Julia_set#Quadratic_polynomials
			"-0.5125 + 0.5213i":       &fractal{ kind: "julia", name: "-0.5125 + 0.5213i",       params: parameters{cReal: -0.5125,        cImag:  0.5213                  }},
			"-0.5251993 - 0.5251993i": &fractal{ kind: "julia", name: "-0.5251993 - 0.5251993i", params: parameters{cReal: -0.5251993,     cImag: -0.5251993               }},
		},
	}

	setZoomPoints(fractals)
	setTitles(fractals)
	setMaxIter(fractals)

	return fractals
}

func setZoomPoints(fractals map[string]map[string]*fractal) {
	for _, fractal := range fractals["julia"] { // locate interesting target coordinates to zoom in on, with some exceptions
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

func setTitles(fractals map[string]map[string]*fractal) {
	for kind, fractalsByName := range fractals {
		for _, fractal := range fractalsByName {
			displayName := fractal.name
			if strings.ContainsAny(string(displayName[0]), "+-0") { // hack to avoid using regexp
				displayName = "unnamed"
			}
			switch kind {
			case "mandelbrot":
				fractal.titleText = fmt.Sprintf("%s - %s (%v, %vi)", kind, displayName, fractal.params.xReal, fractal.params.yImag)
			case "julia":
				displayCReal := fmt.Sprintf("%v", fractal.params.cReal) // hack to avoid using strconv
				displayCImag := fmt.Sprintf("%v", fractal.params.cImag)
				if fractal.name == "golden" { // special handle "golden" since φ is irrational, hence its decimal representation is long
					displayCReal = "(φ - 2)"
					displayCImag = "(φ - 1)"
				}
				fractal.titleText = fmt.Sprintf("%s - %s (c = %s + %si)", kind, displayName, displayCReal, displayCImag)
			}
		}
	}
}

func setMaxIter(fractals map[string]map[string]*fractal) {
	for kind, fractalsByName := range fractals {
		for _, fractal := range fractalsByName {
			switch kind {
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
}
