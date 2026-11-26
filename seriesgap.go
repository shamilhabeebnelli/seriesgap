// Package main implements seriesgap: find gaps, duplicates and order breaks in numbered series
// such as invoice numbers. Each series is the text around the last number in a value.
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
)

var lastNum = regexp.MustCompile(`^(.*?)(\d+)(\D*)$`)

// Entry is one parsed value.
type Entry struct {
	Raw    string
	Prefix string
	Suffix string
	Num    *big.Int
	Width  int // digits as written
	Line   int
}

// Parse splits a value into series key parts and its number.
func Parse(raw string, line int) (Entry, bool) {
	v := strings.TrimSpace(raw)
	m := lastNum.FindStringSubmatch(v)
	if m == nil {
		return Entry{}, false
	}
	n, ok := new(big.Int).SetString(m[2], 10)
	if !ok {
		return Entry{}, false
	}
	return Entry{Raw: v, Prefix: m[1], Suffix: m[3], Num: n, Width: len(m[2]), Line: line}, true
}

// Range is an inclusive run of numbers.
type Range struct {
	From string `json:"from"`
	To   string `json:"to"`
	Size string `json:"count"`
}

// Dup is a repeated value.
type Dup struct {
	Value string `json:"value"`
	Lines []int  `json:"lines"`
}

// Break is a value that appears lower than one before it in file order.
type Break struct {
	Value string `json:"value"`
	Line  int    `json:"line"`
	After string `json:"after"`
}

// SeriesReport is the result for one series.
type SeriesReport struct {
	Series     string  `json:"series"`
	First      string  `json:"first"`
	Last       string  `json:"last"`
	Used       int     `json:"unique_values"`
	Missing    []Range `json:"missing"`
	MissingNum string  `json:"missing_total"`
	Duplicates []Dup   `json:"duplicates"`
	OutOfOrder []Break `json:"out_of_order"`
	WidthMixed bool    `json:"mixed_zero_padding"`
}

// Report is the whole result.
type Report struct {
	Series     []SeriesReport `json:"series"`
	Unreadable []string       `json:"unreadable"`
}

// Options control the analysis.
type Options struct {
	Excused map[string]bool // values to treat as accounted for (cancelled, voided)
}

func fmtNum(prefix, suffix string, n *big.Int, width int) string {
	s := n.String()
	for len(s) < width {
		s = "0" + s
	}
	return prefix + s + suffix
}

// Analyze builds the report from values in file order.
func Analyze(values []string, opt Options) Report {
	groups := map[string][]Entry{}
	var order []string
	var rep Report
	for i, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		e, ok := Parse(v, i+1)
		if !ok {
			rep.Unreadable = append(rep.Unreadable, fmt.Sprintf("line %d: %q", i+1, strings.TrimSpace(v)))
			continue
		}
		k := e.Prefix + "\x00" + e.Suffix
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}
	for _, k := range order {
		es := groups[k]
		parts := strings.SplitN(k, "\x00", 2)
		rep.Series = append(rep.Series, analyzeSeries(parts[0], parts[1], es, opt))
	}
	if rep.Series == nil {
		rep.Series = []SeriesReport{}
	}
	if rep.Unreadable == nil {
		rep.Unreadable = []string{}
	}
	return rep
}

func analyzeSeries(prefix, suffix string, es []Entry, opt Options) SeriesReport {
	r := SeriesReport{Series: prefix + "#" + suffix, Missing: []Range{}, Duplicates: []Dup{}, OutOfOrder: []Break{}}
	// canonical width: the widest padding seen, used to print missing numbers
	// Only zero-padded numbers set a width; plain numbers print without padding.
	width := 0
	for _, e := range es {
		if len(e.Raw) > len(prefix) && e.Raw[len(prefix)] == '0' && e.Width > 1 && e.Width > width {
			width = e.Width
		}
	}
	excused := map[string]bool{}
	for raw := range opt.Excused {
		if x, ok := Parse(raw, 0); ok && x.Prefix == prefix && x.Suffix == suffix {
			excused[x.Num.String()] = true
		}
	}
	// mixed padding: same number written with different widths where one is zero-padded
	byNum := map[string][]Entry{}
	for i, e := range es {
		kn := e.Num.String()
		byNum[kn] = append(byNum[kn], e)
		if i > 0 {
			p := es[i-1]
			if p.Width != e.Width && (strings.HasPrefix(p.Raw[len(prefix):], "0") || strings.HasPrefix(e.Raw[len(prefix):], "0")) {
				r.WidthMixed = true
			}
		}
	}
	// duplicates: same number (so "INV-007" and "INV-7" count as repeats of one number)
	var keys []string
	for k := range byNum {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := new(big.Int).SetString(keys[i], 10)
		b, _ := new(big.Int).SetString(keys[j], 10)
		return a.Cmp(b) < 0
	})
	for _, k := range keys {
		g := byNum[k]
		if len(g) > 1 {
			d := Dup{Value: g[0].Raw}
			for _, e := range g {
				d.Lines = append(d.Lines, e.Line)
			}
			r.Duplicates = append(r.Duplicates, d)
		}
	}
	// order breaks, file order
	hi := es[0]
	for _, e := range es[1:] {
		if e.Num.Cmp(hi.Num) < 0 {
			r.OutOfOrder = append(r.OutOfOrder, Break{Value: e.Raw, Line: e.Line, After: hi.Raw})
		} else {
			hi = e
		}
	}
	// gaps across sorted unique numbers
	first, _ := new(big.Int).SetString(keys[0], 10)
	last, _ := new(big.Int).SetString(keys[len(keys)-1], 10)
	r.First = fmtNum(prefix, suffix, first, width)
	r.Last = fmtNum(prefix, suffix, last, width)
	r.Used = len(keys)
	total := new(big.Int)
	one := big.NewInt(1)
	for i := 0; i+1 < len(keys); i++ {
		a, _ := new(big.Int).SetString(keys[i], 10)
		b, _ := new(big.Int).SetString(keys[i+1], 10)
		from := new(big.Int).Add(a, one)
		to := new(big.Int).Sub(b, one)
		if from.Cmp(to) > 0 {
			continue
		}
		// remove excused numbers from the run
		cur := new(big.Int).Set(from)
		var runStart *big.Int
		flush := func(end *big.Int) {
			if runStart == nil {
				return
			}
			cnt := new(big.Int).Add(new(big.Int).Sub(end, runStart), one)
			r.Missing = append(r.Missing, Range{From: fmtNum(prefix, suffix, runStart, width), To: fmtNum(prefix, suffix, end, width), Size: cnt.String()})
			total.Add(total, cnt)
			runStart = nil
		}
		span := new(big.Int).Add(new(big.Int).Sub(to, from), one)
		if len(excused) == 0 || span.Cmp(big.NewInt(1000000)) > 0 {
			runStart = from
			flush(to)
			continue
		}
		for cur.Cmp(to) <= 0 {
			if excused[cur.String()] {
				prev := new(big.Int).Sub(cur, one)
				flush(prev)
			} else if runStart == nil {
				runStart = new(big.Int).Set(cur)
			}
			cur.Add(cur, one)
		}
		flush(to)
	}
	r.MissingNum = total.String()
	return r
}

// ReadValues reads one value per line, or a CSV column when col is set (name or 1-based number).
func ReadValues(in io.Reader, col string) ([]string, error) {
	if col == "" {
		b, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		return strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), nil
	}
	cr := csv.NewReader(in)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("the CSV is empty")
	}
	idx := -1
	if n, err := fmt.Sscanf(col, "%d", new(int)); err == nil && n == 1 && regexp.MustCompile(`^\d+$`).MatchString(col) {
		var k int
		fmt.Sscanf(col, "%d", &k)
		idx = k - 1
	} else {
		for i, h := range rows[0] {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")), col) {
				idx = i
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("no column named %q; the header has: %s", col, strings.Join(rows[0], ", "))
		}
		rows = rows[1:]
	}
	if idx < 0 {
		return nil, fmt.Errorf("column numbers start at 1")
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if idx < len(r) {
			out = append(out, r[idx])
		} else {
			out = append(out, "")
		}
	}
	return out, nil
}

// Issues counts problems found.
func (r Report) Issues() int {
	n := len(r.Unreadable)
	for _, s := range r.Series {
		n += len(s.Missing) + len(s.Duplicates) + len(s.OutOfOrder)
	}
	return n
}

// Text renders the report for people.
func (r Report) Text() string {
	var b strings.Builder
	for _, s := range r.Series {
		fmt.Fprintf(&b, "Series %s\n  %s to %s, %d distinct numbers\n", strings.Replace(s.Series, "#", "<n>", 1), s.First, s.Last, s.Used)
		if len(s.Missing) == 0 {
			b.WriteString("  Missing: none\n")
		} else {
			fmt.Fprintf(&b, "  Missing: %s numbers\n", s.MissingNum)
			for _, m := range s.Missing {
				if m.From == m.To {
					fmt.Fprintf(&b, "    %s\n", m.From)
				} else {
					fmt.Fprintf(&b, "    %s to %s (%s)\n", m.From, m.To, m.Size)
				}
			}
		}
		for _, d := range s.Duplicates {
			fmt.Fprintf(&b, "  Repeated: %s on lines %v\n", d.Value, d.Lines)
		}
		for _, o := range s.OutOfOrder {
			fmt.Fprintf(&b, "  Out of order: %s on line %d comes after %s\n", o.Value, o.Line, o.After)
		}
		if s.WidthMixed {
			b.WriteString("  Note: zero padding differs between neighbouring values\n")
		}
	}
	for _, u := range r.Unreadable {
		fmt.Fprintf(&b, "Unreadable (no number): %s\n", u)
	}
	if r.Issues() == 0 {
		b.WriteString("No gaps, repeats or order breaks found.\n")
	}
	return b.String()
}

// JSON renders the report as JSON.
func (r Report) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b) + "\n"
}
