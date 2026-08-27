package main

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/gogpu/gg/text"
)

// adapted from https://github.com/gogpu/gg/blob/main/examples/gogpu_integration/main.go

// loadFont finds a font on the host system and returns the font source.
func loadFont(fsys fs.StatFS) (*text.FontSource, error) {
	path, err := findFont(fsys)
	if err != nil {
		return nil, err
	}

	source, err := text.NewFontSourceFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load font %s: %v", path, err)
	}

	return source, nil
}

// findFont returns the path to a TTF font found on the host system.
func findFont(fsys fs.StatFS) (string, error) {
	for _, path := range fonts() {
		// https://pkg.go.dev/io/fs#hdr-Path_Names
		//
		//   Paths must not start or end with a slash: “/x” and “x/” are invalid.
		if _, err := fs.Stat(fsys, strings.TrimPrefix(path, "/")); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no font found")
}

// fonts returns a slice of paths to commonly-installed TTF fonts.
func fonts() []string {
	return []string{ // halfhearted attempt at portability
		// macOS
		"/System/Library/Fonts/Supplemental/Verdana.ttf",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/SFNS.ttf",
		"/System/Library/Fonts/Monaco.ttf",
		// Linux
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		// Windows
		`C:\Windows\Fonts\arial.ttf`,
		`C:\Windows\Fonts\calibri.ttf`,
		`C:\Windows\Fonts\segoeui.ttf`,
	}
}
