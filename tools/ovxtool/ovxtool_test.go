// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "GLIBC_2.2.5", true},
		{"GLIBC_2.3", "GLIBC_2.3.2", true},
		{"GLIBC_2.9", "GLIBC_2.28", true},
		{"GLIBC_2.28", "GLIBC_2.28", false},
		{"GLIBC_2.34", "GLIBC_2.28", false},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q,%q) = %v", c.a, c.b, got)
		}
	}
}

func TestSplitInfo(t *testing.T) {
	sem, inf := splitInfo(`step=GetErrorText info.text="a \"q\" b" rc=0`)
	if sem != "step=GetErrorText rc=0" || inf != `info.text="a \"q\" b"` {
		t.Fatalf("sem=%q inf=%q", sem, inf)
	}
	sem, inf = splitInfo(`step=InitEx info.log=3`)
	if sem != "" || inf != "step=InitEx info.log=3" {
		t.Fatalf("pure-info record: sem=%q inf=%q", sem, inf)
	}
}

func TestConformDiff(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ref := write("ref.txt", "case=a step=X rc=0 info.log=9\ncase=b step=Y rc=1000\ncase=c status=SIGSEGV\n")
	cand := write("cand.txt", "case=a step=X rc=0 info.log=1\ncase=b step=Y rc=1000\ncase=c step=Z rc=3\ncase=c status=exit:0\n")
	ok := write("ok.json", `[{"case":"c","class":"RETURN_CODE_DIFFERENCE","deviation":"intentional safety improvement","reason":"no crash"}]`)
	if err := cmdConformDiff([]string{"-expect", ok, ref, cand}); err != nil {
		t.Fatalf("expected pass: %v", err)
	}
	none := write("none.json", `[]`)
	if err := cmdConformDiff([]string{"-expect", none, ref, cand}); err == nil {
		t.Fatal("unexplained difference must fail")
	}
	stale := write("stale.json", `[{"case":"c","class":"RETURN_CODE_DIFFERENCE","deviation":"intentional safety improvement","reason":"x"},{"case":"zz","class":"LOG_ONLY_DIFFERENCE","deviation":"implementation limitation","reason":"y"}]`)
	if err := cmdConformDiff([]string{"-expect", stale, ref, cand}); err == nil {
		t.Fatal("stale expectation must fail")
	}
	data := write("data.json", `[{"case":"c","class":"DATA_DIFFERENCE","deviation":"implementation limitation","reason":"x"}]`)
	if err := cmdConformDiff([]string{"-expect", data, ref, cand}); err == nil {
		t.Fatal("data differences must never be accepted")
	}
}
