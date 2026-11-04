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
	"cmp"
	"fmt"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/orijtech/structslop"
)

// finding is a structslop.Finding with its position resolved.
type finding struct {
	structslop.Finding
	File      string // display path
	Line, Col int
}

func (f finding) location() string {
	return fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Col)
}

// Sorting.

var sortFields = []string{"filename", "structname", "oldsize", "newsize", "percent"}

func comparePosition(a, b finding) int {
	return cmp.Or(cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col))
}

// parseSort parses the -sort flag value into a comparison function.
func parseSort(v string) (func(a, b finding) int, error) {
	if v == "" {
		return comparePosition, nil
	}
	desc := strings.HasPrefix(v, "-")
	field := strings.TrimPrefix(v, "-")
	var byField func(a, b finding) int
	switch field {
	case "filename":
		byField = comparePosition
	case "structname":
		byField = func(a, b finding) int { return cmp.Compare(a.Name, b.Name) }
	case "oldsize":
		byField = func(a, b finding) int { return cmp.Compare(a.OldSize, b.OldSize) }
	case "newsize":
		byField = func(a, b finding) int { return cmp.Compare(a.NewSize, b.NewSize) }
	case "percent":
		byField = func(a, b finding) int { return cmp.Compare(a.Savings, b.Savings) }
	default:
		return nil, fmt.Errorf("invalid -sort value %q: must be one of %s, optionally prefixed with '-'",
			v, strings.Join(sortFields, ", "))
	}
	return func(a, b finding) int {
		c := byField(a, b)
		if desc {
			c = -c
		}
		// Ties keep file position order.
		return cmp.Or(c, comparePosition(a, b))
	}, nil
}

// Paths.

// pathDisplay shortens absolute file names relative to the
// directories named on the command line.
type pathDisplay struct {
	base string // common parent of the directory arguments
	cwd  string
}

func newPathDisplay(patterns []string, cwd string) pathDisplay {
	pd := pathDisplay{cwd: cwd}
	var dirs []string
	for _, p := range patterns {
		p = strings.TrimSuffix(p, "...")
		if p == "" {
			p = "."
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		p = filepath.Clean(p)
		fi, err := os.Stat(p)
		if err != nil {
			continue // an import path, not a directory
		}
		if !fi.IsDir() {
			p = filepath.Dir(p)
		}
		dirs = append(dirs, p)
	}
	// With several directories, use their common parent so that files
	// with the same name in different directories stay distinguishable.
	for i, d := range dirs {
		if i == 0 {
			pd.base = d
			continue
		}
		for !within(pd.base, d) {
			parent := filepath.Dir(pd.base)
			if parent == pd.base {
				break
			}
			pd.base = parent
		}
	}
	return pd
}

// relWithin returns path relative to dir, if path is inside dir.
func relWithin(dir, path string) (string, bool) {
	r, err := filepath.Rel(dir, path)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return r, true
}

func within(dir, path string) bool {
	_, ok := relWithin(dir, path)
	return ok
}

// rel returns name relative to the directory arguments, falling back to
// the working directory, and then to name itself.
func (pd pathDisplay) rel(name string) string {
	for _, dir := range []string{pd.base, pd.cwd} {
		if dir == "" {
			continue
		}
		if r, ok := relWithin(dir, name); ok {
			return r
		}
	}
	return name
}

// Colors.

// palette holds the escape sequences used for each kind of output.
// The zero value prints plain text.
type palette struct {
	reset    string
	location string
	header   string
	keyword  string
	field    string
	typ      string
	pkg      string
	number   string
	str      string
	comment  string
}

var (
	plain = palette{}
	ansi  = palette{
		reset:    "\x1b[0m",
		location: "\x1b[1m",
		header:   "\x1b[1m",
		keyword:  "\x1b[35m",
		field:    "\x1b[33m",
		typ:      "\x1b[36m",
		pkg:      "\x1b[34m",
		number:   "\x1b[32m",
		str:      "\x1b[32m",
		comment:  "\x1b[90m",
	}
)

func (p palette) paint(code, s string) string {
	if code == "" || s == "" {
		return s
	}
	return code + s + p.reset
}

func parseColor(v string, out io.Writer) (bool, error) {
	switch v {
	case "on":
		return true, nil
	case "off":
		return false, nil
	case "auto":
		if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
			return false, nil
		}
		f, ok := out.(*os.File)
		if !ok {
			return false, nil
		}
		fi, err := f.Stat()
		return err == nil && fi.Mode()&os.ModeCharDevice != 0, nil
	}
	return false, fmt.Errorf("invalid -color value %q: must be on, off or auto", v)
}

// highlight returns src, a gofmt'd struct type, with Go syntax
// highlighting applied.
func highlight(src string, p palette) string {
	if p == plain {
		return src
	}
	type tok struct {
		off, end int
		tok      token.Token
		line     int
	}
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	var s scanner.Scanner
	s.Init(file, []byte(src), nil, scanner.ScanComments)
	var toks []tok
	for {
		pos, t, lit := s.Scan()
		if t == token.EOF {
			break
		}
		if t == token.SEMICOLON && lit == "\n" {
			continue // automatically inserted
		}
		n := len(lit)
		if n == 0 {
			n = len(t.String())
		}
		off := file.Offset(pos)
		toks = append(toks, tok{off, off + n, t, file.Line(pos)})
	}

	var b strings.Builder
	last := 0
	firstOnLine := -1
	for i, t := range toks {
		b.WriteString(src[last:t.off])
		text := src[t.off:t.end]
		if i == 0 || toks[i-1].line != t.line {
			firstOnLine = i
		}
		var code string
		switch {
		case t.tok.IsKeyword():
			code = p.keyword
		case t.tok == token.IDENT:
			next := token.EOF
			if i+1 < len(toks) && toks[i+1].line == t.line {
				next = toks[i+1].tok
			}
			switch {
			case next == token.PERIOD:
				code = p.pkg
			case i == firstOnLine && next != token.EOF && next != token.SEMICOLON && next != token.RBRACE:
				code = p.field // "name type" at the start of a line
			default:
				code = p.typ
			}
		case t.tok == token.INT || t.tok == token.FLOAT || t.tok == token.IMAG:
			code = p.number
		case t.tok == token.STRING || t.tok == token.CHAR:
			code = p.str
		case t.tok == token.COMMENT:
			code = p.comment
		}
		b.WriteString(p.paint(code, text))
		last = t.end
	}
	b.WriteString(src[last:])
	return b.String()
}

// Printing.

// printFull prints each finding as "file:line:col: message", followed by
// the suggested struct layout.
func printFull(w io.Writer, fs []finding, p palette) {
	for _, f := range fs {
		fmt.Fprintf(w, "%s: %s\n", p.paint(p.location, f.location()), f.Header)
		if f.Optimal != "" {
			fmt.Fprintf(w, "%s\n\n", highlight(f.Optimal, p))
		}
	}
}

// printTable prints one row per finding.
func printTable(w io.Writer, fs []finding, p palette) {
	header := []string{"FILE", "STRUCT", "OLD SIZE", "NEW SIZE", "SAVINGS"}
	rightAlign := []bool{false, false, true, true, true}
	rows := make([][]string, 0, len(fs))
	for _, f := range fs {
		rows = append(rows, []string{
			fmt.Sprintf("%s:%d", f.File, f.Line),
			f.Name,
			strconv.FormatInt(f.OldSize, 10),
			strconv.FormatInt(f.NewSize, 10),
			fmt.Sprintf("%.2f%%", f.Savings),
		})
	}
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, c := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(c))
		}
	}
	line := func(cells []string, code string) {
		var b strings.Builder
		for i, c := range cells {
			pad := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c))
			if i > 0 {
				b.WriteString("  ")
			}
			switch {
			case rightAlign[i]:
				b.WriteString(pad)
				b.WriteString(p.paint(code, c))
			case i == len(cells)-1:
				b.WriteString(p.paint(code, c))
			default:
				b.WriteString(p.paint(code, c))
				b.WriteString(pad)
			}
		}
		fmt.Fprintln(w, b.String())
	}
	line(header, p.header)
	for _, r := range rows {
		line(r, "")
	}
}
