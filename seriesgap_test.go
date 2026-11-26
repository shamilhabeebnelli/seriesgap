package main

import (
	"bytes"
	"strings"
	"testing"
)

func lines(s string) []string { return strings.Split(s, "\n") }

func TestGapsAndRanges(t *testing.T) {
	r := Analyze(lines("INV-001\nINV-002\nINV-005\nINV-006\nINV-010"), Options{})
	s := r.Series[0]
	if len(r.Series) != 1 || s.First != "INV-001" || s.Last != "INV-010" || s.MissingNum != "5" {
		t.Fatalf("bad series: %+v", s)
	}
	if len(s.Missing) != 2 || s.Missing[0].From != "INV-003" || s.Missing[0].To != "INV-004" || s.Missing[1].From != "INV-007" || s.Missing[1].To != "INV-009" {
		t.Fatalf("bad missing: %+v", s.Missing)
	}
}

func TestSeparateSeriesByPrefix(t *testing.T) {
	r := Analyze(lines("A/24-25/1\nA/24-25/2\nB/24-25/1\nB/24-25/3\nA/25-26/1"), Options{})
	if len(r.Series) != 3 {
		t.Fatalf("want 3 series, got %d", len(r.Series))
	}
	if r.Series[1].MissingNum != "1" || r.Series[1].Missing[0].From != "B/24-25/2" {
		t.Fatalf("bad B series: %+v", r.Series[1])
	}
	if r.Series[0].MissingNum != "0" || r.Series[2].MissingNum != "0" {
		t.Fatal("A series should have no gaps")
	}
}

func TestDuplicatesAndPaddingVariants(t *testing.T) {
	r := Analyze(lines("INV-007\nINV-008\nINV-8\nINV-009\nINV-007"), Options{})
	s := r.Series[0]
	if len(s.Duplicates) != 2 {
		t.Fatalf("want 2 repeated numbers, got %+v", s.Duplicates)
	}
	if !s.WidthMixed {
		t.Fatal("expected mixed padding note")
	}
	if len(s.Missing) != 0 {
		t.Fatalf("no gaps expected: %+v", s.Missing)
	}
}

func TestOutOfOrder(t *testing.T) {
	r := Analyze(lines("1\n2\n4\n3\n5"), Options{})
	s := r.Series[0]
	if len(s.OutOfOrder) != 1 || s.OutOfOrder[0].Value != "3" || s.OutOfOrder[0].After != "4" || s.OutOfOrder[0].Line != 4 {
		t.Fatalf("bad order break: %+v", s.OutOfOrder)
	}
	if len(s.Missing) != 0 {
		t.Fatal("3 is present, so no gap")
	}
}

func TestExcusedNumbers(t *testing.T) {
	ex := map[string]bool{"INV-003": true, "INV-004": true, "INV-007": true}
	r := Analyze(lines("INV-001\nINV-002\nINV-005\nINV-006\nINV-008\nINV-011"), Options{Excused: ex})
	s := r.Series[0]
	if s.MissingNum != "2" || len(s.Missing) != 1 || s.Missing[0].From != "INV-009" || s.Missing[0].To != "INV-010" {
		t.Fatalf("excused handling wrong: %+v", s)
	}
}

func TestExcusedSplitsRun(t *testing.T) {
	r := Analyze(lines("1\n10"), Options{Excused: map[string]bool{"5": true}})
	s := r.Series[0]
	if len(s.Missing) != 2 || s.Missing[0].To != "4" || s.Missing[1].From != "6" || s.MissingNum != "7" {
		t.Fatalf("run not split: %+v", s)
	}
}

func TestUnreadableAndBlank(t *testing.T) {
	r := Analyze(lines("INV-1\n\nhello\nINV-2"), Options{})
	if len(r.Unreadable) != 1 || !strings.Contains(r.Unreadable[0], "line 3") {
		t.Fatalf("unreadable: %+v", r.Unreadable)
	}
	if r.Series[0].MissingNum != "0" {
		t.Fatal("no gap expected")
	}
}

func TestHugeNumbersDoNotOverflow(t *testing.T) {
	r := Analyze(lines("99999999999999999999999\n100000000000000000000002"), Options{})
	s := r.Series[0]
	if s.MissingNum != "2" || s.Missing[0].From != "100000000000000000000000" {
		t.Fatalf("big numbers: %+v", s)
	}
}

func TestCSVColumn(t *testing.T) {
	csv := "date,Invoice No,amount\n2026-01-01,INV-1,10\n2026-01-02,INV-3,20\n"
	v, err := ReadValues(strings.NewReader(csv), "invoice no")
	if err != nil || len(v) != 2 || v[1] != "INV-3" {
		t.Fatalf("name col: %v %v", v, err)
	}
	v, err = ReadValues(strings.NewReader(csv), "2")
	if err != nil || len(v) != 3 || v[0] != "Invoice No" {
		t.Fatalf("numeric col includes header row: %v %v", v, err)
	}
	if _, err = ReadValues(strings.NewReader(csv), "nope"); err == nil || !strings.Contains(err.Error(), "Invoice No") {
		t.Fatalf("missing column message: %v", err)
	}
}

func TestRunExitCodesAndFormats(t *testing.T) {
	var out bytes.Buffer
	code, err := run(nil, "", "", "text", strings.NewReader("1\n2\n3\n"), &out)
	if err != nil || code != 0 || !strings.Contains(out.String(), "No gaps") {
		t.Fatalf("clean: %d %v %q", code, err, out.String())
	}
	out.Reset()
	code, _ = run(nil, "", "", "json", strings.NewReader("1\n3\n"), &out)
	if code != 1 || !strings.Contains(out.String(), `"missing_total": "1"`) {
		t.Fatalf("json: %d %q", code, out.String())
	}
	if _, err := run(nil, "", "", "xml", strings.NewReader(""), &out); err == nil {
		t.Fatal("bad format should error")
	}
}
