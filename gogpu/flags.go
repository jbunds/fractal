package main

import (
	"flag"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"strings"
)

// params represents a named preset set of parameters:
//   - the real and imaginary (x, y) coordinates of a target point of interest in the Mandelbrot set; or
//   - the real and imaginary components of the complex term (𝑐) which defines the filled Julia set
type params struct {
	name         string
	cReal, cImag float64
	xReal, yImag float64
}

// paramsOfInterest returns a map of named preset parameters used to
// render a fractal and zoom in on interesting locations therein.
func paramsOfInterest() map[string]map[string]params {
	params := map[string]map[string]params{
		"mandelbrot": {
			"spiral":    {name: "spiral",           xReal: -0.088,             yImag: 0.6555           }, // default
			"elephant":  {name: "elephant valley",  xReal:  0.2777,            yImag: 0.0073           },
			"lightning": {name: "lightning branch", xReal: -1.250662834,       yImag: 0.020126938      },
			"seahorse":  {name: "seahorse valley",  xReal: -0.743643887037151, yImag: 0.131825904205330},
			"scepter":   {name: "scepter",          xReal: -1.45,              yImag: 0.0              },
			"spider":    {name: "spider",           xReal: -1.4063,            yImag: 0.0              },
			"starburst": {name: "starburst",        xReal: -0.77568377,        yImag: 0.13646737       },
		},
		"julia": {
			"spiral":        {name: "spiral galaxy", cReal: -0.8,           cImag:  0.156        }, // default
			"basilica":      {name: "basilica",      cReal: -0.75,          cImag:  0.0          },
			"cantor":        {name: "cantor dust",   cReal:  1.0,           cImag:  0.0          }, // TODO(jbunds): improve the rendering of this fractal
			"dendrite":      {name: "dendrite",      cReal:  0.0,           cImag:  1.0          },
			"golden":        {name: "golden",        cReal: math.Phi - 2.0, cImag: math.Phi - 1.0},
			"rabbit":        {name: "rabbit",        cReal: -0.122561,      cImag:  0.744862     },
			"siegel":        {name: "siegel",        cReal: -0.390541,      cImag: -0.586788     },
			// most of these were taken from
			// https://e.math.cornell.edu/people/belk/dynamicalsystems/NotesJuliaMandelbrot.pdf
			// and i'm aware of no associated canonical names for any of these complex constants
			"+0.37 + 0.16i": {name: "+0.37 + 0.16i", cReal:  0.37,          cImag:  0.16         },
			"+0.40 + 0.10i": {name: "+0.40 + 0.10i", cReal:  0.40,          cImag:  0.10         },
			"-0.40 + 0.60i": {name: "-0.40 + 0.60i", cReal: -0.40,          cImag:  0.60         },
			"-0.50 - 0.56i": {name: "-0.50 - 0.56i", cReal: -0.50,          cImag: -0.56         },
			"-0.75 + 0.25i": {name: "-0.75 + 0.25i", cReal: -0.75,          cImag:  0.25         }, // TODO(jbunds): improve the rendering of this fractal
			"-1.50 + 0.00i": {name: "-1.50 + 0.00i", cReal: -1.50,          cImag:  0.00         },
		},
	}
	for k, p := range params["julia"] { // locate interesting target coordinates to zoom in on, with some exceptions
		switch k {
		case "siegel",
		     "spiral",
		     "+0.37 + 0.16i",
		     "-1.50 + 0.00i": // zoom in on the critical point (origin) rather than the α fixed point for these fractals
			p.xReal, p.yImag   = 0, 0
			params["julia"][k] = p
		default:
			c := complex(p.cReal, p.cImag)
			z := (1.0 - cmplx.Sqrt(1.0 - 4.0 * c)) / 2.0 // calculate the α fixed point by solving 𝑧² - 𝑧 + 𝑐 = 0
			p.xReal, p.yImag   = real(z), imag(z)
			params["julia"][k] = p
		}
	}
	return params
}

// flags parses command line flags and returns the fractal parameters for the user-specified preset.
func flags(fs *flag.FlagSet, args []string) (string, params, error) {
	args = filterArgs(args)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "%s usage:\n\n", filepath.Base(fs.Name()))
		fs.PrintDefaults()
		_, _ = fmt.Fprintln(fs.Output())
	}
	var targetName, fractalName string
	fs.StringVar(&fractalName, "fractal", "mandelbrot", `fractal ("mandelbrot" or "julia")`)
	fs.StringVar(&targetName,  "target",  "spiral",     `canonical name for target coordinates or complex term ("seahorse", "dendrite", etc)`)
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return "", params{}, err
	}
	parameters        := paramsOfInterest()
	fractalParams, ok := parameters[fractalName]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -fractal:\n\n")
		for fractalName := range parameters {
			fmt.Fprintf(os.Stderr, "  %s\n", fractalName)
		}
		fmt.Fprintln(os.Stderr)
		return "", params{}, fmt.Errorf("invalid fractal specified: %q", fractalName)
	}
	flagSpecified := make(map[string]bool, 2)
	flag.Visit(func(f *flag.Flag) { flagSpecified[f.Name] = true })
	if fractalName == "julia" && !flagSpecified["target"] {
		targetName = "spiral" // set default for the `-fractal julia` case
	}
	target, ok := fractalParams[targetName]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -target:\n\n")
		for targetName := range fractalParams {
			fmt.Fprintf(os.Stderr, "  %s\n", targetName)
		}
		fmt.Fprintln(os.Stderr)
		return "", params{}, fmt.Errorf("invalid target specified: %q", targetName)
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(fs.Output(), "ignored arguments: %s\n", strings.Join(fs.Args(), ", "))
	}
	return fractalName, target, nil
}

// filterArgs discards any arguments up to and including "--".
func filterArgs(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			args = args[i + 1:]
			break
		}
	}
	return args
}
