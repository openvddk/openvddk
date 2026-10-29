// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Command ovxtool holds the project's ABI and conformance tooling:
//
//	ovxtool elf-facts LIB                 machine-readable ELF/ABI facts
//	ovxtool abi-gate -manifest M -reference R -libdir DIR
//	ovxtool layout-check -expected E CANDIDATE.json
//	ovxtool conform-diff -expect X REFERENCE.txt CANDIDATE.txt
//	ovxtool ledger -manifest M            render COMPATIBILITY.md
//	ovxtool defer-go-init LIB             start the Go runtime at the first call, not at load
//	ovxtool tracediff REF.trace CAND.trace compare two vixtrace call traces
//	ovxtool compat-header [-abi H]        render include/vixDiskLib.h
//	ovxtool sbom -version V -commit C FILE... SPDX 2.3 SBOM of the release's Go binaries
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	args := os.Args[2:]
	switch os.Args[1] {
	case "elf-facts":
		err = cmdElfFacts(args)
	case "abi-gate":
		err = cmdABIGate(args)
	case "layout-check":
		err = cmdLayoutCheck(args)
	case "conform-diff":
		err = cmdConformDiff(args)
	case "ledger":
		err = cmdLedger(args)
	case "defer-go-init":
		err = cmdDeferGoInit(args)
	case "tracediff":
		err = cmdTraceDiff(args)
	case "compat-header":
		err = cmdCompatHeader(args)
	case "sbom":
		err = cmdSBOM(args)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ovxtool:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ovxtool elf-facts|abi-gate|layout-check|conform-diff|ledger|defer-go-init|tracediff|compat-header|sbom [flags]")
	os.Exit(2)
}
