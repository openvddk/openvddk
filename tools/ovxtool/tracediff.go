// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// tracediff compares two call traces written by tests/trace/vixtrace.c
// (CONTRACT.md §77, catalogue row C1): the same consumer run once against
// each library. Handles and pointers are renamed in order of appearance,
// thread IDs, sequence numbers and times are dropped, and libDir is
// ignored. Control calls are compared in order; reads (Read, ReadAsync
// completions) are compared as a set per handle, because a consumer with
// several threads issues them in no fixed order.

var (
	ptrToken  = regexp.MustCompile(`\b([CHPIL]):(0x[0-9a-f]+|\(nil\))`)
	timeToken = regexp.MustCompile(` t=[0-9.]+$`)
	libDirArg = regexp.MustCompile(`libDir="[^"]*"`)
	readLine  = regexp.MustCompile(`^(?:Read|ReadAsyncDone) (H\d+|H:nil) start=(\d+) n=(\d+) = (\d+) hash=([0-9a-f]+)$`)
)

type readKey struct {
	handle   string
	start, n uint64
}

type readVal struct{ result, hash string }

type trace struct {
	control []string
	reads   map[readKey]readVal
	waits   int
	submits int
}

func parseTrace(r io.Reader) (*trace, error) {
	t := &trace{reads: map[readKey]readVal{}}
	names := map[string]string{}
	count := map[string]int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.SplitN(line, " ", 3) // sequence, thread, call
		if len(f) < 3 {
			return nil, fmt.Errorf("malformed trace line %q", line)
		}
		call := timeToken.ReplaceAllString(f[2], "")
		call = libDirArg.ReplaceAllString(call, `libDir=<ignored>`)
		// A pointer after " = " is returned by the call: it gets a new name
		// even if the same address was used before (memory is reused).
		rename := func(born bool) func(string) string {
			return func(m string) string {
				sub := ptrToken.FindStringSubmatch(m)
				if sub[2] == "(nil)" {
					return sub[1] + ":nil"
				}
				if n, ok := names[m]; ok && !born {
					return n
				}
				count[sub[1]]++
				n := sub[1] + strconv.Itoa(count[sub[1]])
				names[m] = n
				return n
			}
		}
		args, result, hasResult := strings.Cut(call, " = ")
		call = ptrToken.ReplaceAllStringFunc(args, rename(false))
		if hasResult {
			call += " = " + ptrToken.ReplaceAllStringFunc(result, rename(true))
		}
		switch {
		case strings.HasPrefix(call, "Read ") || strings.HasPrefix(call, "ReadAsyncDone "):
			m := readLine.FindStringSubmatch(call)
			if m == nil {
				return nil, fmt.Errorf("malformed read %q", call)
			}
			start, _ := strconv.ParseUint(m[2], 10, 64)
			n, _ := strconv.ParseUint(m[3], 10, 64)
			t.reads[readKey{m[1], start, n}] = readVal{m[4], m[5]}
		case strings.HasPrefix(call, "ReadAsync "):
			t.submits++
		case strings.HasPrefix(call, "Wait "):
			t.waits++
		default:
			t.control = append(t.control, call)
		}
	}
	return t, sc.Err()
}

// diffLines is a longest-common-subsequence diff of two short sequences.
func diffLines(a, b []string) []string {
	n, m := len(a), len(b)
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else if l[i+1][j] >= l[i][j+1] {
				l[i][j] = l[i+1][j]
			} else {
				l[i][j] = l[i][j+1]
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, "  "+a[i])
			i++
			j++
		case l[i+1][j] >= l[i][j+1]:
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, "- "+a[i])
	}
	for ; j < m; j++ {
		out = append(out, "+ "+b[j])
	}
	return out
}

// compareTraces reports the differences; it returns whether there were any.
func compareTraces(w io.Writer, a, b *trace, nameA, nameB string) bool {
	differ := false
	d := diffLines(a.control, b.control)
	changed := 0
	for _, l := range d {
		if !strings.HasPrefix(l, "  ") {
			changed++
		}
	}
	fmt.Fprintf(w, "Control calls: %d in %s, %d in %s; %d differ.\n", len(a.control), nameA, len(b.control), nameB, changed)
	if changed > 0 {
		differ = true
		for _, l := range d {
			fmt.Fprintln(w, l)
		}
	}
	handles := map[string]bool{}
	for k := range a.reads {
		handles[k.handle] = true
	}
	for k := range b.reads {
		handles[k.handle] = true
	}
	var hs []string
	for h := range handles {
		hs = append(hs, h)
	}
	sort.Strings(hs)
	for _, h := range hs {
		var na, nb, sa, sb uint64
		var onlyA, onlyB, results, data int
		for k, va := range a.reads {
			if k.handle != h {
				continue
			}
			na++
			sa += k.n
			vb, ok := b.reads[k]
			switch {
			case !ok:
				onlyA++
			case va.result != vb.result:
				results++
			case va.result == "0" && va.hash != vb.hash:
				data++
			}
		}
		for k := range b.reads {
			if k.handle != h {
				continue
			}
			nb++
			sb += k.n
			if _, ok := a.reads[k]; !ok {
				onlyB++
			}
		}
		fmt.Fprintf(w, "Reads on %s: %d calls, %d sectors in %s; %d calls, %d sectors in %s; only in %s %d, only in %s %d, different result %d, different data %d.\n",
			h, na, sa, nameA, nb, sb, nameB, nameA, onlyA, nameB, onlyB, results, data)
		if onlyA+onlyB+results+data > 0 {
			differ = true
		}
	}
	fmt.Fprintf(w, "Asynchronous reads submitted: %d / %d; Wait calls: %d / %d.\n", a.submits, b.submits, a.waits, b.waits)
	return differ
}

func cmdTraceDiff(args []string) error {
	fs := flag.NewFlagSet("tracediff", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: ovxtool tracediff REFERENCE.trace CANDIDATE.trace")
	}
	var ts [2]*trace
	for i, p := range fs.Args() {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		ts[i], err = parseTrace(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	if compareTraces(os.Stdout, ts[0], ts[1], "reference", "candidate") {
		os.Exit(1)
	}
	return nil
}
