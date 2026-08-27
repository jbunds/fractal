package main

import (
	"flag"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// flags parses command line flags and returns the parameters used to render the user-specified fractal.
func flags(fs *flag.FlagSet, args []string) (*fractal, string, error) {
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
		return nil, "", err
	}
	// TODO(jbunds): refactor the 3x duplicated logic that writes user-friendly messages to fs.Output() below
	fractals := fractals()
	kinds    := kinds(fractals)
	if !slices.Contains(kinds, kind) {
		fs.Usage()
		fmt.Fprintf(fs.Output(), "valid values for -type:\n\n")
		for _, kind := range kinds {
			fmt.Fprintf(fs.Output(), "  %s\n", kind)
		}
		fmt.Fprintln(fs.Output())
		return nil, "", fmt.Errorf("invalid fractal type specified: %q", kind)
	}
	flagSpecified := make(map[string]bool, 3)
	fs.Visit(func(f *flag.Flag) { flagSpecified[f.Name] = true }) // visits only the flags that have been set
	if kind == "julia" && !flagSpecified["fractal"] {
		name = "spiral" // set default for the `-type julia` case
	}
	fractalNames         := fractalNamesByKind(fractals, kind)
	specifiedFractal, ok := fractals[name]
	if !ok || !slices.Contains(slices.Collect(fractalNames), name) {
		fs.Usage()
		fmt.Fprintf(fs.Output(), "valid values for -fractal:\n\n")
		for _, name := range slices.Sorted(fractalNames) {
			fmt.Fprintf(fs.Output(), "  %s\n", name)
		}
		fmt.Fprintln(fs.Output())
		return nil, "", fmt.Errorf("invalid fractal identifier specified: %q", name)
	}
	themes := slices.Sorted(maps.Keys(colorSchemes()))
	if !slices.Contains(themes, theme) {
		fs.Usage()
		fmt.Fprintf(fs.Output(), "valid values for -theme:\n\n")
		for _, theme := range themes {
			fmt.Fprintf(fs.Output(), "  %s\n", theme)
		}
		fmt.Fprintln(fs.Output())
		return nil, "", fmt.Errorf("invalid theme specified: %q", theme)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "ignored arguments: %s\n", strings.Join(fs.Args(), ", "))
	}
	return specifiedFractal, theme, nil
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
