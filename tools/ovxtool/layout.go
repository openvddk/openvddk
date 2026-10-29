// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
)

func readLayout(path string) (map[string]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]int
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// cmdLayoutCheck compares a layout_probe output with the expected layout.
func cmdLayoutCheck(args []string) error {
	fs := flag.NewFlagSet("layout-check", flag.ContinueOnError)
	expectedPath := fs.String("expected", "", "expected layout JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: layout-check -expected E CANDIDATE.json")
	}
	want, err := readLayout(*expectedPath)
	if err != nil {
		return err
	}
	got, err := readLayout(fs.Arg(0))
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bad := 0
	for _, k := range keys {
		g, ok := got[k]
		if !ok || g != want[k] {
			fmt.Printf("FAIL %s: want %d got %d (present=%v)\n", k, want[k], g, ok)
			bad++
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			fmt.Printf("FAIL %s: not in expected layout\n", k)
			bad++
		}
	}
	if bad > 0 {
		return fmt.Errorf("layout-check: %d mismatch(es)", bad)
	}
	fmt.Printf("layout-check: %d values match\n", len(keys))
	return nil
}
