package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const usage = `seriesgap: find missing, repeated and out-of-order numbers in a series such as invoice numbers.

Usage: seriesgap [flags] [file]
  Reads one value per line from the file or standard input, or a CSV column with --col.

Flags:
`

func main() {
	col := flag.String("col", "", "read a CSV column, by header name or 1-based number")
	excuse := flag.String("excuse", "", "file of values (one per line) to treat as accounted for, such as cancelled invoices")
	format := flag.String("format", "text", "text or json")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage); flag.PrintDefaults() }
	flag.Parse()
	code, err := run(flag.Args(), *col, *excuse, *format, os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seriesgap:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run(args []string, col, excuse, format string, stdin io.Reader, stdout io.Writer) (int, error) {
	if format != "text" && format != "json" {
		return 2, fmt.Errorf("--format must be text or json")
	}
	in := stdin
	if len(args) > 1 {
		return 2, fmt.Errorf("give at most one file")
	}
	if len(args) == 1 {
		f, err := os.Open(args[0])
		if err != nil {
			return 2, err
		}
		defer f.Close()
		in = f
	}
	vals, err := ReadValues(in, col)
	if err != nil {
		return 2, err
	}
	opt := Options{Excused: map[string]bool{}}
	if excuse != "" {
		b, err := os.ReadFile(excuse)
		if err != nil {
			return 2, err
		}
		for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				opt.Excused[l] = true
			}
		}
	}
	rep := Analyze(vals, opt)
	if format == "json" {
		fmt.Fprint(stdout, rep.JSON())
	} else {
		fmt.Fprint(stdout, rep.Text())
	}
	if rep.Issues() > 0 {
		return 1, nil
	}
	return 0, nil
}
