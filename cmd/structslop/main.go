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

// Command structslop reports structs whose fields can be rearranged to
// use less memory.
//
// Unlike a singlechecker-based tool, it collects the findings for all
// packages before printing them, so they can be sorted and printed as a
// table.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/orijtech/structslop"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("structslop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	brief := fs.Bool("brief", false, "print only a table of file, struct, old size, new size and savings")
	sortBy := fs.String("sort", "", "sort findings by `field`: "+strings.Join(sortFields, ", ")+
		"\n(prefix with '-' for descending order, e.g. -sort=-percent; default: file position)")
	color := fs.String("color", "auto", "colorize output: on, off or auto (on when stdout is a terminal)")
	structslop.Analyzer.Flags.VisitAll(func(f *flag.Flag) {
		fs.Var(f.Value, f.Name, f.Usage)
	})
	fs.Usage = func() {
		fmt.Fprintf(stderr, "%s\n\nUsage: structslop [flags] [packages]\n\nFlags:\n", structslop.Doc)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	less, err := parseSort(*sortBy)
	if err != nil {
		fmt.Fprintf(stderr, "structslop: %v\n", err)
		return 2
	}
	useColor, err := parseColor(*color, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "structslop: %v\n", err)
		return 2
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	findings, ok := analyze(patterns, fs.Lookup("include-test-files").Value.String() == "true", stderr)

	cwd, _ := os.Getwd()
	paths := newPathDisplay(patterns, cwd)
	for i := range findings {
		findings[i].File = paths.rel(findings[i].File)
	}
	slices.SortStableFunc(findings, less)

	p := plain
	if useColor {
		p = ansi
	}
	if *brief {
		printTable(stdout, findings, p)
	} else {
		printFull(stdout, findings, p)
	}

	switch {
	case !ok:
		return 1
	case len(findings) > 0:
		return 3 // same convention as singlechecker
	}
	return 0
}

// analyze loads the packages matching patterns, runs the analyzer on
// them and returns the de-duplicated findings. ok is false if any
// package could not be loaded or analyzed; the errors are written to
// stderr.
func analyze(patterns []string, tests bool, stderr io.Writer) (findings []finding, ok bool) {
	cfg := &packages.Config{Mode: packages.LoadSyntax, Tests: tests}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintf(stderr, "structslop: %v\n", err)
		return nil, false
	}
	ok = true
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			fmt.Fprintln(stderr, err)
			ok = false
		}
	})

	// Skip generated test main packages ("p.test").
	roots := pkgs[:0]
	for _, pkg := range pkgs {
		if !strings.HasSuffix(pkg.ID, ".test") {
			roots = append(roots, pkg)
		}
	}
	if len(roots) == 0 {
		fmt.Fprintf(stderr, "structslop: no packages matched %s\n", strings.Join(patterns, " "))
		return nil, false
	}

	graph, err := checker.Analyze([]*analysis.Analyzer{structslop.Analyzer}, roots, nil)
	if err != nil {
		fmt.Fprintf(stderr, "structslop: %v\n", err)
		return nil, false
	}

	// With -include-test-files a file can belong to both "p" and
	// "p [p.test]", so drop repeated findings.
	type key struct {
		file      string
		line, col int
	}
	seen := make(map[key]bool)
	for _, act := range graph.Roots {
		if act.Err != nil {
			fmt.Fprintf(stderr, "structslop: %s: %v\n", act.Package.ID, act.Err)
			ok = false
			continue
		}
		res, _ := act.Result.(*structslop.Result)
		if res == nil {
			continue
		}
		for _, f := range res.Findings {
			pos := act.Package.Fset.Position(f.Pos)
			k := key{pos.Filename, pos.Line, pos.Column}
			if seen[k] {
				continue
			}
			seen[k] = true
			findings = append(findings, finding{Finding: f, File: pos.Filename, Line: pos.Line, Col: pos.Column})
		}
	}
	return findings, ok
}
