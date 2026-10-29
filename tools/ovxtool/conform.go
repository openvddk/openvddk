// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

// Difference classes (CONTRACT.md section 62).
var diffClasses = map[string]bool{
	"DATA_DIFFERENCE":         true,
	"RETURN_CODE_DIFFERENCE":  true,
	"ABI_DIFFERENCE":          true,
	"TRANSPORT_DIFFERENCE":    true,
	"EXTENT_DIFFERENCE":       true,
	"EXPECTED_NONDETERMINISM": true,
	"PERFORMANCE_DIFFERENCE":  true,
	"LOG_ONLY_DIFFERENCE":     true,
}

// Deviation classes (CONTRACT.md section 99).
var deviationClasses = map[string]bool{
	"intentional safety improvement":                true,
	"implementation limitation":                     true,
	"unsupported feature":                           true,
	"known incompatibility":                         true,
	"reference implementation quirk not reproduced": true,
	"performance difference":                        true,
}

// Expectation accepts a documented difference for matching cases.
type Expectation struct {
	Case      string `json:"case"` // path.Match pattern
	Class     string `json:"class"`
	Deviation string `json:"deviation"`
	Reason    string `json:"reason"`
	Ledger    string `json:"ledger,omitempty"`
}

type transcript struct {
	order []string
	recs  map[string][]string // case -> normalized non-informational records
	info  map[string][]string // case -> informational records
}

func parseTranscript(p string) (*transcript, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t := &transcript{recs: map[string][]string{}, info: map[string][]string{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "case=") {
			continue
		}
		name, rest, _ := strings.Cut(strings.TrimPrefix(line, "case="), " ")
		if _, seen := t.recs[name]; !seen {
			t.order = append(t.order, name)
			t.recs[name] = nil
		}
		semantic, informational := splitInfo(rest)
		if semantic != "" {
			t.recs[name] = append(t.recs[name], semantic)
		}
		if informational != "" {
			t.info[name] = append(t.info[name], informational)
		}
	}
	return t, sc.Err()
}

// splitInfo separates tokens whose key starts with "info." from the rest.
// Values may be double-quoted with backslash escapes.
func splitInfo(s string) (semantic, informational string) {
	var sem, inf []string
	for _, tok := range tokens(s) {
		if strings.HasPrefix(tok, "info.") {
			inf = append(inf, tok)
		} else {
			sem = append(sem, tok)
		}
	}
	// A record whose only semantic token is its step label is purely
	// informational.
	if len(sem) == 1 && strings.HasPrefix(sem[0], "step=") && len(inf) > 0 {
		inf = append([]string{sem[0]}, inf...)
		sem = nil
	}
	return strings.Join(sem, " "), strings.Join(inf, " ")
}

func tokens(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote, esc := false, false
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && inQuote:
			cur.WriteRune(r)
			esc = true
		case r == '"':
			cur.WriteRune(r)
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func loadExpectations(p string) ([]Expectation, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var xs []Expectation
	if err := json.Unmarshal(data, &xs); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	for _, x := range xs {
		if _, err := path.Match(x.Case, "probe"); err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", x.Case, err)
		}
		if !diffClasses[x.Class] || !deviationClasses[x.Deviation] || x.Reason == "" {
			return nil, fmt.Errorf("expectation %q: class, deviation and reason are required", x.Case)
		}
		if x.Class == "DATA_DIFFERENCE" {
			return nil, fmt.Errorf("expectation %q: data differences can never be accepted", x.Case)
		}
	}
	return xs, nil
}

func matchExpectation(xs []Expectation, name string) *Expectation {
	for i := range xs {
		if ok, _ := path.Match(xs[i].Case, name); ok {
			return &xs[i]
		}
	}
	return nil
}

// cmdConformDiff compares reference and candidate transcripts. Every
// semantic difference must be covered by a documented expectation.
func cmdConformDiff(args []string) error {
	fs := flag.NewFlagSet("conform-diff", flag.ContinueOnError)
	expectPath := fs.String("expect", "", "accepted differences (JSON)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: conform-diff -expect X REFERENCE.txt CANDIDATE.txt")
	}
	xs, err := loadExpectations(*expectPath)
	if err != nil {
		return err
	}
	ref, err := parseTranscript(fs.Arg(0))
	if err != nil {
		return err
	}
	cand, err := parseTranscript(fs.Arg(1))
	if err != nil {
		return err
	}
	names := append([]string{}, ref.order...)
	for _, n := range cand.order {
		if _, ok := ref.recs[n]; !ok {
			names = append(names, n)
		}
	}

	var match, expected, unexplained, logOnly int
	used := map[string]bool{}
	for _, n := range names {
		r, inRef := ref.recs[n]
		c, inCand := cand.recs[n]
		same := inRef && inCand && strings.Join(r, "\n") == strings.Join(c, "\n")
		if same {
			match++
			if strings.Join(ref.info[n], "\n") != strings.Join(cand.info[n], "\n") {
				logOnly++
			}
			continue
		}
		if x := matchExpectation(xs, n); x != nil {
			expected++
			used[x.Case] = true
			fmt.Printf("EXPECTED   %-36s %s / %s\n", n, x.Class, x.Deviation)
			continue
		}
		unexplained++
		fmt.Printf("UNEXPLAINED %s\n", n)
		fmt.Printf("  reference: %s\n", strings.Join(r, " | "))
		fmt.Printf("  candidate: %s\n", strings.Join(c, " | "))
	}
	var stale []string
	for _, x := range xs {
		if !used[x.Case] {
			stale = append(stale, x.Case)
		}
	}
	sort.Strings(stale)
	for _, s := range stale {
		fmt.Printf("STALE      expectation %q matched no difference\n", s)
	}
	fmt.Printf("conform-diff: cases=%d match=%d expected=%d unexplained=%d log-only=%d stale=%d\n",
		len(names), match, expected, unexplained, logOnly, len(stale))
	if unexplained > 0 || len(stale) > 0 {
		return fmt.Errorf("conform-diff: %d unexplained difference(s), %d stale expectation(s)", unexplained, len(stale))
	}
	return nil
}
