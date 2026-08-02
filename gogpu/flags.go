package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// coords represents the x, y position of a target point of interest.
type coords struct {
	name string
	x, y float64
}

// flags parses command line flags.
func flags(fs *flag.FlagSet, args []string) (coords, error) {
	args = filterArgs(args)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "%s usage:\n\n", filepath.Base(fs.Name()))
		fs.PrintDefaults()
		fmt.Fprintln(fs.Output())
	}
	presets := map[string]coords{
		"seahorse":  {name: "seahorse",  x: -0.743643887037151, y: 0.131825904205330},
		"spiral":    {name: "spiral",    x: -0.088,             y: 0.6555           },
		"elephant":  {name: "elephant",  x:  0.2777,            y: 0.0073           },
		"spider":    {name: "spider",    x: -1.4063,            y: 0.0              },
		"lightning": {name: "lightning", x: -1.25066,           y: 0.02012          },
		"scepter":   {name: "scepter",   x: -1.45,              y: 0.0              },
	}
	name := "spiral" // default
	fs.StringVar(&name, "target", "spiral", "canonical name for target x, y coordinates (seahorse, spiral, etc)")
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return coords{}, err
	}
	target, ok := presets[name]
	if !ok {
		fs.Usage()
		fmt.Fprintf(os.Stderr, "valid values for -target:\n\n")
		for name := range presets {
			fmt.Fprintf(os.Stderr, "  %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
		return coords{}, fmt.Errorf("invalid target specified: %q", name)
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(fs.Output(), "ignored arguments: %s\n", strings.Join(fs.Args(), ", "))
	}
	return target, nil
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
