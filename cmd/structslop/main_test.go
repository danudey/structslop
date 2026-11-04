// Copyright 2020 Orijtech, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/orijtech/structslop"
)

const structDir = "../../testdata/src/struct"

func runCmd(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestRunFull(t *testing.T) {
	out, stderr, code := runCmd(t, "-color", "off", structDir)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr:\n%s", code, stderr)
	}
	want := "p.go:30:9: struct has size 24 (size class 24), could be 16 (size class 16), you'll save 33.33% if you rearrange it to:\n" +
		"struct {\n\ty uint64\n\tx uint32\n\tz uint32\n}\n\n" +
		"p.go:36:9: "
	if !strings.HasPrefix(out, want) {
		t.Errorf("output starts with:\n%s\nwant prefix:\n%s", out[:min(len(out), len(want))], want)
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("output contains escape sequences with -color off")
	}
}

func TestRunBriefSorted(t *testing.T) {
	out, stderr, code := runCmd(t, "-color", "off", "-brief", "-sort", "-percent", structDir)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr:\n%s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if got := strings.Fields(lines[0]); !slices.Equal(got, []string{"FILE", "STRUCT", "OLD", "SIZE", "NEW", "SIZE", "SAVINGS"}) {
		t.Errorf("header = %q", lines[0])
	}
	var names []string
	for _, l := range lines[1:] {
		names = append(names, strings.Fields(l)[1])
	}
	// 50% first, then 33.33% and 25%; ties stay in file order.
	want := []string{"s4", "s9", "s10", "s3", "s7", "s8", "s5"}
	if !slices.Equal(names, want) {
		t.Errorf("struct order = %q, want %q", names, want)
	}
	if !regexp.MustCompile(`^p\.go:36 +s4 +40 +24 +50\.00%$`).MatchString(lines[1]) {
		t.Errorf("unexpected first row %q", lines[1])
	}
}

func TestRunColorOn(t *testing.T) {
	out, _, _ := runCmd(t, "-color", "on", structDir)
	if !strings.Contains(out, ansi.keyword+"struct"+ansi.reset) {
		t.Errorf("expected highlighted struct keyword in:\n%s", out)
	}
}

func TestRunBadFlags(t *testing.T) {
	for _, args := range [][]string{{"-sort", "bogus"}, {"-color", "sometimes"}} {
		if _, _, code := runCmd(t, append(args, structDir)...); code != 2 {
			t.Errorf("%q: exit code = %d, want 2", args, code)
		}
	}
}

func TestParseSort(t *testing.T) {
	fs := []finding{
		{Finding: structslop.Finding{Name: "b", OldSize: 40, NewSize: 24, Savings: 50}, File: "a.go", Line: 2},
		{Finding: structslop.Finding{Name: "c", OldSize: 24, NewSize: 16, Savings: 33}, File: "a.go", Line: 1},
		{Finding: structslop.Finding{Name: "a", OldSize: 32, NewSize: 32, Savings: 25}, File: "b.go", Line: 1},
	}
	tests := map[string][]string{
		"":            {"c", "b", "a"},
		"filename":    {"c", "b", "a"},
		"-filename":   {"a", "b", "c"},
		"structname":  {"a", "b", "c"},
		"oldsize":     {"c", "a", "b"},
		"-newsize":    {"a", "b", "c"},
		"percent":     {"a", "c", "b"},
		"-percent":    {"b", "c", "a"},
		"-structname": {"c", "b", "a"},
	}
	for v, want := range tests {
		cmpFn, err := parseSort(v)
		if err != nil {
			t.Fatalf("parseSort(%q): %v", v, err)
		}
		s := slices.Clone(fs)
		slices.SortStableFunc(s, cmpFn)
		var got []string
		for _, f := range s {
			got = append(got, f.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("-sort=%s: got %q, want %q", v, got, want)
		}
	}
}

func TestPathDisplay(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "mod")
	for _, d := range []string{"mod/pkg/foo", "mod/pkg/bar", "other"} {
		mkdir(t, filepath.Join(root, d))
	}
	file := filepath.Join(cwd, "pkg/foo/bar.go")
	tests := []struct {
		patterns []string
		want     string
	}{
		{[]string{"."}, "pkg/foo/bar.go"},
		{[]string{"./..."}, "pkg/foo/bar.go"},
		{[]string{"./pkg/..."}, "foo/bar.go"},
		{[]string{"./pkg/foo"}, "bar.go"},
		{[]string{"./pkg/foo", "./pkg/bar"}, "foo/bar.go"},
		{[]string{cwd + "/..."}, "pkg/foo/bar.go"},
		{[]string{"example.com/some/import/path"}, "pkg/foo/bar.go"}, // falls back to cwd
	}
	for _, tt := range tests {
		if got := newPathDisplay(tt.patterns, cwd).rel(file); got != tt.want {
			t.Errorf("%q: rel = %q, want %q", tt.patterns, got, tt.want)
		}
	}
	// A file outside both the arguments and the working directory stays absolute.
	outside := filepath.Join(root, "other", "x.go")
	if got := newPathDisplay([]string{"./pkg/foo"}, cwd).rel(outside); got != outside {
		t.Errorf("rel(%q) = %q, want it unchanged", outside, got)
	}
}

func TestHighlight(t *testing.T) {
	src := "struct {\n\t_  [0]func()\n\tt  *httptest.Server\n\tm  map[string]int `json:\"m\"`\n\tEmbedded\n}"
	got := highlight(src, ansi)
	for _, want := range []string{
		ansi.keyword + "struct" + ansi.reset,
		ansi.field + "_" + ansi.reset,
		ansi.number + "0" + ansi.reset,
		ansi.keyword + "func" + ansi.reset,
		ansi.pkg + "httptest" + ansi.reset,
		ansi.typ + "Server" + ansi.reset,
		ansi.typ + "string" + ansi.reset,
		ansi.str + "`json:\"m\"`" + ansi.reset,
		ansi.typ + "Embedded" + ansi.reset,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%q", want, got)
		}
	}
	stripped := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(got, "")
	if stripped != src {
		t.Errorf("highlighting changed the text:\n%s\nwant:\n%s", stripped, src)
	}
	if highlight(src, plain) != src {
		t.Error("plain palette changed the text")
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
