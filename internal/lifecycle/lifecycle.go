// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package lifecycle implements the process-global VixDiskLib_Init /
// VixDiskLib_InitEx / VixDiskLib_Exit state (CONTRACT.md section 12).
//
// Behavior pinned by reference observation (docs/observations/LIFE-*):
// see the Init doc comment. Anything not yet observed is marked
// PROVISIONAL and covered by the differential harness.
package lifecycle

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/openvddk/openvddk/internal/buildinfo"
	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/transport"
	"github.com/openvddk/openvddk/internal/vix"
)

// Options are the InitEx arguments.
type Options struct {
	Major, Minor uint32
	LibDir       string
	ConfigFile   string // "" when NULL
	Legacy       bool   // VixDiskLib_Init rather than InitEx
}

// State is the global library state.
type State struct {
	mu          sync.Mutex
	initialized bool
	opts        Options
	cfg         *config.Config
}

// Global is the single process-wide instance.
var Global = &State{cfg: config.Empty()}

// Supported reports whether a client built for major.minor can be served.
// Clients request the API version they were compiled against; this
// implementation provides generation 8.0, so newer requests are refused.
func Supported(major, minor uint32) bool {
	return major < 8 || (major == 8 && minor == 0)
}

// Init initializes or re-initializes the library.
//
//   - client versions above 8.0 fail with VIX_E_NOT_SUPPORTED;
//   - a repeated Init/InitEx while initialized succeeds and replaces the
//     configuration (PROVISIONAL);
//   - a configuration file that cannot be read is logged and ignored
//     (PROVISIONAL).
func (s *State) Init(o Options) vix.Code {
	if !Supported(o.Major, o.Minor) {
		logx.Warnf("client requested VixDiskLib %d.%d; this library provides 8.0", o.Major, o.Minor)
		return vix.ENotSupported
	}
	cfg := config.Empty()
	var cfgErr error
	if o.ConfigFile != "" {
		if loaded, err := config.Load(o.ConfigFile); err != nil {
			cfgErr = err
		} else {
			cfg = loaded
		}
	}
	// The library's own log files start first, so the configuration
	// warnings and the banner reach them too (D-0022). VMware's
	// tmpDirectory places the own log file, and its two log level keys
	// hold messages back by level (D-0049).
	tmpDir, _ := cfg.Get("tmpDirectory")
	logx.SetOwnDir(tmpDir)
	transportLevel, transportBad := vmwareLevel(cfg, "vixDiskLib.transport.LogLevel", 6)
	nfcLevel, nfcBad := vmwareLevel(cfg, "vixDiskLib.nfc.LogLevel", 4)
	logx.SetLevels(transportLevel, nfcLevel)
	logFile, _ := cfg.Get("OpenVDDK.trace.file")
	logErr := logx.Start(logFile)
	for _, bad := range []string{transportBad, nfcBad} {
		if bad != "" {
			logx.Warnf("configuration: %s", bad)
		}
	}
	if cfgErr != nil {
		logx.Warnf("configuration file not used: %v", cfgErr)
	}
	for _, w := range cfg.Warnings {
		logx.Warnf("configuration: %s", w)
	}
	for _, k := range cfg.NotHonoured() {
		logx.Warnf("configuration: %s has no effect in this library: %s", k[0], k[1])
	}
	if logErr != nil {
		logx.Warnf("OpenVDDK.trace.file %q not used: %v", logFile, logErr)
	}

	s.mu.Lock()
	again := s.initialized
	s.initialized = true
	s.opts = o
	s.cfg = cfg
	s.mu.Unlock()

	if again {
		logx.Infof("re-initialized for client %d.%d", o.Major, o.Minor)
	} else {
		logx.Infof("%s", buildinfo.String())
		logx.Infof("initialized for client %d.%d; available transport modes: %s", o.Major, o.Minor, transport.ListModes())
	}
	return vix.OK
}

// Exit releases global state. Calling it when not initialized is harmless.
func (s *State) Exit() {
	s.mu.Lock()
	was := s.initialized
	s.initialized = false
	s.cfg = config.Empty()
	s.mu.Unlock()
	if was {
		logx.Exit.Infof("exit")
	}
	logx.Stop()
}

// Initialized reports whether Init has succeeded since the last Exit.
func (s *State) Initialized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initialized
}

// Config returns the active configuration.
func (s *State) Config() *config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// vmwareLevel reads one of VMware's log level keys (0 to max); -1 when the
// file does not set it, and also, with a warning text, when the value is
// not a level.
func vmwareLevel(cfg *config.Config, key string, max int) (int, string) {
	v, ok := cfg.Get(key)
	if !ok {
		return -1, ""
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > max {
		return -1, fmt.Sprintf("%s = %q is not 0 to %d; every message is delivered", key, v, max)
	}
	return n, ""
}
