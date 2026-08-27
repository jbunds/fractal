package main

import (
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

type mockFS struct {
	fstest.MapFS
}

func (m mockFS) Stat(path string) (fs.FileInfo, error) {
	// https://pkg.go.dev/io/fs#hdr-Path_Names
	//
	//   Paths must not start or end with a slash: “/x” and “x/” are invalid.
	return fs.Stat(m.MapFS, strings.TrimPrefix(path, "/"))
}

func TestFindFont(t *testing.T) {
	t.Parallel()
	tests := []struct{
		name    string
		path    string  
		want    string
		wantErr bool
	}{{
		name:    "included in list of expected fonts",
		path:    "/System/Library/Fonts/SFNS.ttf", // must match one of the hard-coded paths in candidateFonts()
	}, {
		name:    "excluded from list of expected fonts",
		path:    "/not/included/in/the/list",
		wantErr: true,
	}, {
		name:    "no expected fonts found",
		wantErr: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			absPathFS := fstest.MapFS{}
			if tt.path != "" {
				absPathFS = fstest.MapFS{ strings.TrimPrefix(tt.path, "/"): {} }
			}
			mfs      := &mockFS{MapFS: absPathFS}
			got, err := findFont(mfs)

			if (err != nil) != tt.wantErr {
				t.Fatalf("findFont() = %v; wantErr = %t", err, tt.wantErr)
			}

			if tt.wantErr { return }

			paths := fonts()
			if !slices.Contains(paths, got) {
				t.Errorf("findFont() = %q; expected one of %+v\n", got, paths)
			}

			want := tt.path
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("findFont() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
