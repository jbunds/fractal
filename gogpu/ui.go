package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// labels holds strings for UI elements keyed off the params stru
type labels struct {
	menuItemText string
	windowTitle  string
}

// isUnnamed returns true if the parameter has no associated canonical name.
func isUnnamed(name string) bool {
	return strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-")
}

// normalizeName returns "unnamed" if the name starts with + or -, original name otherwise.
func normalizeName(name string) string {
	if isUnnamed(name) {
		return "unnamed"
	}
	return name
}

// TODO(jbunds): refactor everything below

// uiLabels creates formatted and sorted lists of labels for UI elements as maps keyed off the given "params" struct.
func uiLabels(params map[string]map[string]params) (map[string]map[string]labels, map[string][]string) {
	maxLen       := make(map[string]int)
	fractalTypes := slices.Sorted(maps.Keys(params))

	for _, fractalType := range fractalTypes {
		for _, p := range params[fractalType] {
			maxLen[fractalType] = max(maxLen[fractalType], len(normalizeName(p.name)))
		}
	}

	format := map[string]map[string]string{
		"menu": {
			"mandelbrot": fmt.Sprintf("%%-%ds  %%v, %%vi", maxLen["mandelbrot"] + 1),
			"julia":      fmt.Sprintf("%%-%ds  c = %%v + %%vi", maxLen["julia"] + 1),
		},
		"window": {
			"mandelbrot": "%s - %s (%v, %vi)",
			"julia":      "%s - %s (c = %v + %vi)",
		},
	}

	labelEntries    := make(map[string]map[string]labels, len(params)) // fractal-specific formatted menu item text and window title
	sortedMenuItems := make(map[string][]string)                       // fractal-type-specific sorted list of menu items

	for _, fractalType := range fractalTypes {
		labelEntries[fractalType] = make(map[string]labels)
		
		keys := slices.SortedFunc(maps.Keys(params[fractalType]), func(a, b string) int {
			prioA := 0; if isUnnamed(params[fractalType][a].name) { prioA = 1 }
			prioB := 0; if isUnnamed(params[fractalType][b].name) { prioB = 1 }
			if prioA != prioB { return prioA - prioB }
			return strings.Compare(a, b)
		})

		sortedMenuItems[fractalType] = keys

		for _, key := range keys {
			p           := params[fractalType][key]
			displayName := normalizeName(p.name)
			
			switch fractalType {
			case "mandelbrot":
				displayXReal := fmt.Sprintf("%v", p.xReal) // hack...
				if !strings.HasPrefix(displayXReal, "-") {
					displayXReal = " " + fmt.Sprintf("%v", p.xReal)
				}
				labelEntries[fractalType][key] = labels{
					menuItemText: fmt.Sprintf(format["menu"  ][fractalType], displayName + ":",    displayXReal, p.yImag),
					windowTitle:  fmt.Sprintf(format["window"][fractalType], fractalType, displayName,  p.xReal, p.yImag),
				}
			case "julia":
				displayCReal := fmt.Sprintf("%v", p.cReal) // hack...
				if !strings.HasPrefix(displayCReal, "-") {
					displayCReal = " " + fmt.Sprintf("%v", p.cReal)
				}
				labelEntries[fractalType][key] = labels{
					menuItemText: fmt.Sprintf(format["menu"  ][fractalType], displayName + ":",    displayCReal, p.cImag),
					windowTitle:  fmt.Sprintf(format["window"][fractalType], fractalType, displayName,  p.cReal, p.cImag),
				}
			}
		}
	}

	return labelEntries, sortedMenuItems
}
