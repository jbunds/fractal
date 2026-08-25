package main

import (
	"image/color"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func toRGBA(t *testing.T, val uint32) color.RGBA {
	t.Helper()
	return color.RGBA{
		R: uint8( val        & 0xFF),
		G: uint8((val >>  8) & 0xFF),
		B: uint8((val >> 16) & 0xFF),
		A: uint8((val >> 24) & 0xFF),
	}
}

func TestInitPalette(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name  string
		theme string
		want  []color.RGBA
	}{
		{
			name:  "green",
			theme: "green",
			want:  []color.RGBA{
				{R:  25, G:  30, B:  28, A: 255},
				{R: 245, G: 240, B: 225, A: 255},
				{R:  25, G:  30, B:  28, A: 255},
			},
		},
		{
			name:  "red",
			theme: "red",
			want:  []color.RGBA{
				{R:  40, G:   5, B:   5, A: 255},
				{R: 255, G: 255, B: 245, A: 255},
				{R:  40, G:   5, B:   5, A: 255},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got     := initPalette(tt.theme)
			gotRGBA := []color.RGBA{
				toRGBA(t, got[0]),
				toRGBA(t, got[paletteSize / 2]),
				toRGBA(t, got[paletteSize - 1]),
			}
			for i, c := range gotRGBA {
				if diff := cmp.Diff(tt.want[i], c); diff != "" {
					t.Errorf("initPalette(%q) mismatch (-want +got):\n%s", tt.theme, diff)
				}
			}
		})
	}
}
