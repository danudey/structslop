# structslop

![Build status](https://github.com/orijtech/structslop/workflows/Go/badge.svg?branch=master)

Package structslop defines an [Analyzer](analyzer_link) that checks if struct fields can be re-arranged to optimize size.

## Installation

With Go modules:

```sh
go get github.com/orijtech/structslop/cmd/structslop
```

Without Go modules:

```sh
$ cd $GOPATH/src/github.com/orijtech/structslop
$ git checkout v0.0.6
$ go get
$ install ./cmd/structslop
```

## Usage

You can run `structslop` either on a Go package or Go files, the same way as
other Go tools work.

Example:

```sh
$ structslop github.com/orijtech/structslop/testdata/src/struct
```

or:

```sh
$ structslop ./testdata/src/struct/p.go
```

Sample output:

```text
p.go:30:9: struct has size 24 (size class 32), could be 16 (size class 16), you'll save 50.00% if you rearrange it to:
struct {
	y uint64
	x uint32
	z uint32
}

p.go:36:9: struct has size 40 (size class 48), could be 24 (size class 32), you'll save 33.33% if you rearrange it to:
struct {
	_  [0]func()
	i1 int
	i2 int
	a3 [3]bool
	b  bool
}

p.go:59:9: struct has size 40 (size class 48), could be 32 (size class 32), you'll save 33.33% if you rearrange it to:
struct {
	y uint64
	t *httptest.Server
	w uint64
	x uint32
	z uint32
}

p.go:67:9: struct has size 40 (size class 48), could be 32 (size class 32), you'll save 33.33% if you rearrange it to:
struct {
	y uint64
	t *s
	w uint64
	x uint32
	z uint32
}

```

Example, for the first report above, the output meaning:

 - The current struct size is `24`, the size that the Go runtime will allocate for that struct is `32`.
 - The optimal struct size is `16`, the size that the Go runtime will allocate for that struct is `16`.
 - The layout of optimal struct.
 - The percentage savings with new struct layout.
 
That said, some structs may have a smaller size, but for efficiency, the Go runtime will allocate them in the same size class,
then those structs are not considered sloppy:

```go
type s1 struct {
	x uint32
	y uint64
	z *s
	t uint32
}
```

and:

```go
type s2 struct {
	y uint64
	z *s
	x uint32
	t uint32
}
```

have the same size class `32`, though `s2` layout is only `24` byte in size.

However, you can still get this information when you want, using `-verbose` flag:

```sh
$ structslop -verbose ./testdata/src/verbose/p.go
p.go:17:8: struct has size 0 (size class 0)
p.go:19:9: struct has size 1 (size class 8)
p.go:23:9: struct has size 32 (size class 32), could be 24 (size class 32), optimal fields order:
struct {
	y uint64
	z *s
	x uint32
	t uint32
}
```
 
**Note**

For applying suggested fix, use `-apply` flag, instead of `-fix`.

### File names

File names are printed relative to the directory given on the command line.
For example, `structslop ./...` prints `pkg/foo/bar.go`, and
`structslop ./pkg/...` prints `foo/bar.go`. With several directories, names
are relative to their common parent. For import path arguments, names are
relative to the working directory.

### Summary table

Use `-brief` to print one line per struct instead of the full report:

```sh
$ structslop -brief ./testdata/src/struct
FILE     STRUCT  OLD SIZE  NEW SIZE  SAVINGS
p.go:30  s3            24        16   33.33%
p.go:36  s4            40        24   50.00%
...
```

`OLD SIZE` and `NEW SIZE` are struct sizes in bytes. `SAVINGS` is computed from
the runtime size classes, the same as in the full report.

### Sorting

Use `-sort` to sort the full report or the `-brief` table by one of
`filename`, `structname`, `oldsize`, `newsize` or `percent`. Add a `-` prefix
to sort in descending order. Ties keep file order.

```sh
$ structslop -brief -sort=-percent ./...
```

### Color

`-color=auto` (the default) highlights the suggested struct layout when stdout
is a terminal and `NO_COLOR` is not set. Use `-color=on` or `-color=off` to
force it.

### Exit status

`0` when nothing is reported, `3` when at least one struct is reported, `1` on
load or analysis errors and `2` on invalid flags. Reports are printed to
stdout, errors to stderr.

## Development

Go 1.27+

### Running test

Add test case to `testdata/src/struct` directory, then run:

```shell script
go test
```

## Contributing

TODO

[analyzer_link]: https://pkg.go.dev/golang.org/x/tools/go/analysis#Analyzer
