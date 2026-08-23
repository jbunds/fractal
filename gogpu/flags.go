package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// flags parses command line flags and returns the parameters used to render the user-specified fractal.
func flags(fs *flag.FlagSet, args []string) (fractal, string, error) {
	args     = filterArgs(args)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "%s usage:\n\n", filepath.Base(fs.Name()))
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output())
	}
	var kind, name, theme string
	fs.StringVar(&kind,  "type",    "mandelbrot", `fractal type ("mandelbrot" or "julia")`)
	fs.StringVar(&name,  "fractal", "elephant",   `canonical fractal name ("seahorse", "dendrite", etc)`)
	fs.StringVar(&theme, "theme",   "green",      `color scheme ("green" or "red")`)
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return fractal{}, "", err
	}
	// TODO(jbunds): refactor the 3x duplicated logic that writes user-friendly messages to os.Stderr below
	kinds        := fractals()
	fractals, ok := kinds[kind]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -type:\n\n")
		for kind := range kinds {
			fmt.Fprintf(os.Stderr, "  %s\n", kind)
		}
		fmt.Fprintln(os.Stderr)
		return fractal{}, "", fmt.Errorf("invalid fractal type specified: %q", kind)
	}
	flagSpecified := make(map[string]bool, 3)
	flag.Visit(func(f *flag.Flag) { flagSpecified[f.Name] = true })
	if kind == "julia" && !flagSpecified["fractal"] {
		name = "spiral" // set default for the `-fractal julia` case
	}
	specifiedFractal, ok := fractals[name]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -fractal:\n\n")
		for name := range fractals {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
		return fractal{}, "", fmt.Errorf("invalid fractal specified: %q", name)
	}
	themes := slices.Sorted(maps.Keys(colorSchemes))
	if !slices.Contains(themes, theme) {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -theme:\n\n")
		for _, theme := range themes {
			fmt.Fprintf(os.Stderr, "  %s\n", theme)
		}
		fmt.Fprintln(os.Stderr)
		return fractal{}, "", fmt.Errorf("invalid theme speciied: %q", theme)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "ignored arguments: %s\n", strings.Join(fs.Args(), ", "))
	}
	return *specifiedFractal, theme, nil
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
