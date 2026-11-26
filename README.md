# seriesgap

<img src="logo.svg" width="72" alt="seriesgap logo">

Find missing, repeated and out-of-order numbers in a series such as invoice numbers. One small Go program, no dependencies, works offline. Your data is read from a file or standard input and never leaves your machine.

Tax audits and books often need an unbroken invoice series. Checking a long sales register by eye misses things. `seriesgap` reads the invoice number column, groups numbers into series by their prefix, and lists every gap.

There are web pages that do this in a browser. This one is a command line tool you can run on a register export, in a script, or in a check that fails when a gap appears (exit code 1).

## Install

```
go install github.com/shamilhabeebnelli/seriesgap@latest
```

or clone and run `go build`.

## Use

```
seriesgap invoices.txt                         # one number per line
seriesgap --col "Invoice No" register.csv      # a CSV column, by header name
seriesgap --col 2 register.csv                 # by column number (starts at 1; the header row is read as a value, so prefer the name)
seriesgap --excuse cancelled.txt register.csv --col "Invoice No"
seriesgap --format json register.csv --col "Invoice No"
```

Example output:

```
Series INV/25-26/<n>
  INV/25-26/0001 to INV/25-26/3000, 2952 distinct numbers
  Missing: 48 numbers
    INV/25-26/2057 to INV/25-26/2058 (2)
  Repeated: INV/25-26/0225 on lines [223 2957]
  Out of order: INV/25-26/1146 on line 2953 comes after INV/25-26/3000
```

(Shortened. From a generated 3000-invoice test register with planted gaps and repeats.)

## How it reads a number

A series is the text before and after the last run of digits in a value. `INV/25-26/0007` is series `INV/25-26/<n>` number 7. `INV/26-27/0001` is a different series, so a new financial year does not look like a huge gap. Numbers of any length work.

- Missing: every number between the lowest and highest used number that never appears.
- Repeated: the same number appears more than once. `INV-007` and `INV-7` are the same number.
- Out of order: a number that appears after a higher one in file order.
- `--excuse`: a file of numbers you can account for, such as voided or cancelled invoices, listed one per line. They are not reported as missing.
- Values with no digits are listed as unreadable. Blank lines are ignored.

Exit code: 0 when nothing is found, 1 when there is something to look at, 2 for a usage or file error.

## Limits

- It does not know what your rules say. It reports what is in the list. Whether a gap is a problem, and what may be excused, is for you or your accountant to decide.
- It only checks between the lowest and highest numbers present. It cannot see numbers missing before the first or after the last.
- A series that restarts mid-file with the same prefix is read as one series.
- Tested on generated data, not on real company registers.

## Test

```
go test ./...
```

MIT licensed.
