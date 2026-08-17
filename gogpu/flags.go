package main

import (
	"flag"
	"fmt"
	"math/cmplx"
	"os"
	"path/filepath"
	"strings"
)

// coords represents the named x, y coordinates of a target point of interest.
type coords struct {
	name         string
	cReal, cImag float64
	x,     y     float64
}

// pointsOfInterest returns a slice of named x, y coordinates of canonical points of interest in the Mandelbrot and filled Julia sets.
func pointsOfInterest() map[string]map[string]coords {
	// TODO(jbunds): find the best-looking coordinates for deep magnification within the precision constraints.
	//               Misiurewicz points are generally interesting points, so find highly-precise coordinates for those
	//               (Fractint or Kalles Fraktaler can maybe help identify those coordinates).
	points := map[string]map[string]coords{
		"mandelbrot": {
			"spiral":    {name: "spiral",           x: -0.088,             y: 0.6555           }, // default
			"elephant":  {name: "elephant valley",  x:  0.2777,            y: 0.0073           },
			"lightning": {name: "lightning branch", x: -1.250662834,       y: 0.020126938      },
			"seahorse":  {name: "seahorse valley",  x: -0.743643887037151, y: 0.131825904205330},
			"scepter":   {name: "scepter",          x: -1.45,              y: 0.0              },
			"spider":    {name: "spider",           x: -1.4063,            y: 0.0              },
			"starburst": {name: "starburst",        x: -0.77568377,        y: 0.13646737       },
		},
		"julia": {
			"spiral":   {name: "spiral",      cReal: -0.8,      cImag:  0.156   }, // default
			"airplane": {name: "airplane",    cReal: -0.12,     cImag:  0.74    },
			"cantor":   {name: "cantor dust", cReal:  0.4,      cImag:  0.1     },
			"dendrite": {name: "dendrite",    cReal: -0.4,      cImag:  0.6     },
			"basilica": {name: "basilica",    cReal: -0.75,     cImag:  0.0     },
			"rabbit":   {name: "rabbit",      cReal: -0.123,    cImag:  0.745   },
			"siegel":   {name: "siegel",      cReal: -0.390541, cImag: -0.586788},
		},
	}
	for k, p := range points["julia"] {
		if k == "siegel" || k == "spiral" {
			p.x, p.y = 0, 0
			points["julia"][k] = p
			continue
		}
		c := complex(p.cReal, p.cImag)
		z := (1.0 - cmplx.Sqrt(1.0 - 4.0 * c)) / 2.0
		p.x, p.y = real(z), imag(z)
		points["julia"][k] = p
	}
	return points
}

// flags parses command line flags and returns the fractal name, and the named x, y coordinates of the user-specified point of interest.
func flags(fs *flag.FlagSet, args []string) (string, coords, error) {
	args = filterArgs(args)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "%s usage:\n\n", filepath.Base(fs.Name()))
		fs.PrintDefaults()
		_, _ = fmt.Fprintln(fs.Output())
	}
	var targetName, fractalName string
	fs.StringVar(&fractalName, "fractal", "mandelbrot", `fractal ("mandelbrot" or "julia")`)
	fs.StringVar(&targetName,  "target",  "spiral",     `canonical name for target x, y coordinates ("seahorse", "spiral", etc)`)
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return "", coords{}, err
	}
	points            := pointsOfInterest()
	fractalPoints, ok := points[fractalName]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -fractal:\n\n")
		for fractalName := range points {
			fmt.Fprintf(os.Stderr, "  %s\n", fractalName)
		}
		fmt.Fprintln(os.Stderr)
		return "", coords{}, fmt.Errorf("invalid fractal specified: %q", fractalName)
	}
	flagSpecified := make(map[string]bool, 2)
	flag.Visit(func(f *flag.Flag) { flagSpecified[f.Name] = true })
	if fractalName == "julia" && !flagSpecified["target"] {
		targetName = "spiral" // set default for the `-fractal julia` case
	}
	target, ok := fractalPoints[targetName]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -target:\n\n")
		for targetName := range fractalPoints {
			fmt.Fprintf(os.Stderr, "  %s\n", targetName)
		}
		fmt.Fprintln(os.Stderr)
		return "", coords{}, fmt.Errorf("invalid target specified: %q", targetName)
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
