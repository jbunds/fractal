package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// TODO(jbunds): refactor labels (at least it's not in the hot path...)

// labels creates formatted and sorted lists of labels for UI elements as maps keyed off the given map.
func labels(fractals map[string]*fractal) (map[string]map[string]string, map[string][]string) {
	maxLen := make(map[string]int)
	for _, fractal := range fractals {
		maxLen[fractal.kind] = max(maxLen[fractal.kind], len(normalizeName(fractal.name)))
	}

	format := map[string]string{ // TODO(jbunds): encapsulate format strings in params.go
		"mandelbrot": fmt.Sprintf("%%-%ds  %%v, %%vi", maxLen["mandelbrot"] + 1),
		"julia":      fmt.Sprintf("%%-%ds  c = %%v + %%vi", maxLen["julia"] + 1),
	}

	labelEntries    := make(map[string]map[string]string, len(fractals)) // fractal-specific formatted menu item text and window title
	sortedMenuItems := make(map[string][]string)                         // fractal-kind-specific sorted list of menu items

	for _, kind := range kinds(fractals) {
		labelEntries[kind]  = make(map[string]string)

		sortedFractalNames := slices.SortedFunc(fractalNamesByKind(fractals, kind), func(a, b string) int {
			// TODO(jbunds): clean this up
			prioA := 0; if isUnnamed(a) { prioA = 1 }
			prioB := 0; if isUnnamed(b) { prioB = 1 }
			if prioA != prioB { return prioA - prioB }
			return cmp.Compare(a, b)
		})

		sortedMenuItems[kind] = sortedFractalNames

		for _, name := range sortedFractalNames {
			fractal     := fractals[name]
			displayName := normalizeName(fractal.name)
			
			switch kind { // TODO(jbunds): encapsulate fractal-kind-specific UI display logic and special case handling in params.go
			case "mandelbrot":
				displayXReal := fmt.Sprintf("%v", fractal.params.xReal) // hack to avoid using strconv
				if !strings.HasPrefix(displayXReal, "-") {
					displayXReal = " " + fmt.Sprintf("%v", fractal.params.xReal)
				}
				labelEntries[kind][name] = fmt.Sprintf(format[kind], displayName + ":", displayXReal, fractal.params.yImag)
			case "julia":
				displayCReal := fmt.Sprintf("%v", fractal.params.cReal) // hack to avoid using strconv
				displayCImag := fmt.Sprintf("%v", fractal.params.cImag)
				if !strings.HasPrefix(displayCReal, "-") {
					displayCReal = " " + fmt.Sprintf("%v", fractal.params.cReal)
				}
				if fractal.name == "golden" { // special handle "golden", since φ is irrational, hence its decimal representation is infinite
					displayCReal = "(φ - 2)"
					displayCImag = "(φ - 1)"
				}
				labelEntries[kind][name] = fmt.Sprintf(format[kind], displayName + ":", displayCReal, displayCImag)
			}
		}
	}

	return labelEntries, sortedMenuItems
}

// normalizeName returns "unnamed" if the name starts with + or -, original name otherwise.
func normalizeName(name string) string {
	if isUnnamed(name) {
		return "unnamed"
	}
	return name
}

// isUnnamed returns true if the parameter has no associated canonical name.
func isUnnamed(name string) bool {
	return strings.HasPrefix(name, "+") ||
	       strings.HasPrefix(name, "-")
}
