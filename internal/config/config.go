// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package config parses the VixDiskLib configuration file passed to
// VixDiskLib_InitEx.
//
// Format: one `name = value` pair per line; the value may be wrapped in
// double quotes; lines starting with '#' and blank lines are ignored.
// Names are matched case-insensitively. Malformed lines and unknown keys
// produce warnings, never failures (CONTRACT.md section 15). Keys owned by
// this project are documented with the public prefix "OpenVDDK." (D-0051);
// the original prefix "OpenVix." names the same keys (D-0021) and is the
// form they are stored under here.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// MaxFileSize bounds memory spent on a configuration file.
const MaxFileSize = 1 << 20

// Prefix is the namespace of project-specific keys.
const Prefix = "openvix."

// AltPrefix spells the same namespace with the project's public name; a key
// written with it is the same key (D-0021).
const AltPrefix = "openvddk."

// canonical maps a lower-case key in AltPrefix to the same key in Prefix.
func canonical(key string) string {
	if strings.HasPrefix(key, AltPrefix) {
		return Prefix + key[len(AltPrefix):]
	}
	return key
}

// Config is an immutable set of configuration values.
type Config struct {
	values   map[string]string // lower-case key -> value
	original map[string]string // lower-case key -> key as written
	Warnings []string
}

// Empty returns a configuration with no values.
func Empty() *Config {
	return &Config{values: map[string]string{}, original: map[string]string{}}
}

// Load reads and parses path. A missing or unreadable file is reported as an
// error; the caller decides whether that is fatal.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileSize {
		return nil, fmt.Errorf("configuration file larger than %d bytes", MaxFileSize)
	}
	return Parse(data), nil
}

// Parse parses configuration text. It never fails; problems become warnings.
func Parse(data []byte) *Config {
	c := Empty()
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), MaxFileSize)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		eq := strings.IndexByte(text, '=')
		if eq <= 0 {
			c.warn("line %d: expected name = value", line)
			continue
		}
		name := strings.TrimSpace(text[:eq])
		value := strings.TrimSpace(text[eq+1:])
		if len(value) >= 2 && value[0] == '"' {
			if end := strings.LastIndexByte(value, '"'); end > 0 {
				value = value[1:end]
			} else {
				c.warn("line %d: unterminated quoted value for %q", line, name)
				continue
			}
		} else if strings.HasPrefix(value, `"`) {
			c.warn("line %d: unterminated quoted value for %q", line, name)
			continue
		} else if hash := strings.IndexByte(value, '#'); hash >= 0 {
			// Outside double quotes '#' starts a comment, as VMware's guide
			// describes for its configuration file (D-0048).
			value = strings.TrimSpace(value[:hash])
		}
		key := canonical(strings.ToLower(name))
		if prev, dup := c.original[key]; dup {
			c.warn("line %d: %q overrides an earlier value for %q", line, name, prev)
		}
		c.values[key] = value
		c.original[key] = name
		if strings.HasPrefix(key, Prefix) && !knownOwnKey(key) {
			c.warn("line %d: unknown project key %q", line, name)
		}
	}
	return c
}

func (c *Config) warn(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

// Get returns the value for name (case-insensitive; "OpenVDDK." and
// "OpenVix." name the same key).
func (c *Config) Get(name string) (string, bool) {
	v, ok := c.values[canonical(strings.ToLower(name))]
	return v, ok
}

// Keys returns the configured keys as written, sorted. Values are not
// included so the list is safe to log.
func (c *Config) Keys() []string {
	out := make([]string, 0, len(c.original))
	for _, k := range c.original {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ownKeys lists every recognised project key (lower case). Unknown keys in
// the project namespace are warned about to catch typos.
var ownKeys = map[string]bool{
	"openvix.log.level":               true,
	"openvix.hotadd.journaldir":       true,
	"openvix.hotadd.hostsafety":       true,
	"openvix.hotadd.readaheadkb":      true,
	"openvix.hotadd.readaheaddepth":   true,
	"openvix.hotadd.configureproxy":   true,
	"openvix.hotadd.reconfigurereads": true,
	"openvix.nbd.inflight":            true,
	"openvix.nbd.readaheadkb":         true,
	"openvix.nbd.readaheaddepth":      true,
	"openvix.nbd.sessions":            true,
	"openvix.trace.file":              true,
}

func knownOwnKey(key string) bool { return ownKeys[key] }

// notHonoured lists the configuration keys VMware documents that this
// library does not act on, with the reason (D-0048, the sweep of VMware's
// programming guide). Keys whose effect matches VMware's library anyway
// (vixDiskLib.linuxSSL.verifyCertificates, which VMware's library ignores
// too) are not listed.
var notHonoured = map[string]string{
	"vixdisklib.disklib.enablecache":      "the library has no such cache; its read-ahead is set with the project's readAhead keys",
	"vixdisklib.ssl.enablesslfips":        "the library has no FIPS mode",
	"vixdisklib.nfc.accepttimeoutms":      "VMware's library showed no effect of it on a connection lost before Open or during a read (D-0049)",
	"vixdisklib.nfc.requesttimeoutms":     "VMware's library showed no consistent effect of it on a connection lost before Open or during a read (D-0049)",
	"vixdisklib.nfcfssrvr.timeoutms":      "its effect was not observed in VMware's library (D-0049)",
	"vixdisklib.nfc.writetimeoutms":       "the library does not write",
	"vixdisklib.nfcfssrvrwrite.timeoutms": "the library does not write",
	"vixdisklib.transport.san.allowlist":  "the SAN transport is not supported",
	"vixdisklib.transport.san.denylist":   "the SAN transport is not supported",
	"vixdisklib.transport.san.whitelist":  "the SAN transport is not supported",
	"vixdisklib.transport.san.blacklist":  "the SAN transport is not supported",
	"vixdisklib.phonehome.productname":    "the library sends no usage data",
	"vixdisklib.phonehome.productversion": "the library sends no usage data",
	"vixdisklib.phonehome.databasedir":    "the library sends no usage data",
}

// NotHonoured returns, for every key VMware documents that is set in the
// file but not acted on, the key as written and the reason, sorted by key.
func (c *Config) NotHonoured() [][2]string {
	var out [][2]string
	for key, why := range notHonoured {
		if name, ok := c.original[key]; ok {
			out = append(out, [2]string{name, why})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i][0]) < strings.ToLower(out[j][0]) })
	return out
}
