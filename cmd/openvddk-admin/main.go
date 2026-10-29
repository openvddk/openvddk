// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Command openvddk-admin inspects and repairs the HotAdd state OpenVDDK
// keeps on a worker (CONTRACT.md §104, §105, §128.4). It lists the HotAdd
// journal and the disks attached to the worker VM, reports what recovery
// would do, runs that recovery with the library's own code, and writes a
// diagnostic bundle without secrets.
//
// It never implements a second cleanup algorithm: reconcile and cleanup
// call internal/hotadd's Assess and Recover, the code behind the library's
// own recovery at Open and VixDiskLib_Cleanup.
package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/openvddk/openvddk/internal/buildinfo"
	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/linuxdev"
	"github.com/openvddk/openvddk/internal/logx"
	"github.com/openvddk/openvddk/internal/vsphere"
)

const usage = `openvddk-admin: inspect and repair OpenVDDK's HotAdd state on this worker

Usage: openvddk-admin <command> [options]

Commands:
  version       print this program's version
  journal       list the HotAdd journal of this worker (no vCenter needed)
  attachments   list the disks attached to this worker VM and match them to the journal
  reconcile     report what recovery would do with each journal record; changes nothing
  cleanup       recover orphaned attachments of this worker, as VixDiskLib_Cleanup does
  bundle        write a diagnostic bundle (no passwords, cookies or keys)

Options (every command that needs them):
  -config FILE         the configuration file the application passes to VixDiskLib_InitEx
  -journal-dir DIR     the journal directory (default: from -config, else /var/lib/openvix/hotadd)
  -server HOST         vCenter address
  -port N              HTTPS port (default 443)
  -user NAME           vCenter user
  -password-file FILE  file whose first line is the password (never pass a password on the command line)
  -thumbprint HEX      SHA-1 or SHA-256 thumbprint of the vCenter certificate (optional)

bundle only:
  -out FILE            the archive to write (default openvddk-bundle-<time>.tar.gz)
  -lib PATH            a library file to describe (repeatable; default: the usual install paths)
  -since DURATION      how far back to collect logs (default 24h)
  -log-lines N         lines kept from the end of each log (default 2000)

Exit status: 0 done, 1 failed, 2 usage, 3 cleanup left records behind.
`

const defaultJournalDir = "/var/lib/openvix/hotadd"

type options struct {
	config, journalDir             string
	server, user, passwordFile, tp string
	port                           uint

	out      string
	libs     multi
	since    time.Duration
	logLines int
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	// Messages of shared packages go to stderr, never to the library's own
	// log file.
	logx.SetSink(func(_ logx.Level, msg string) logx.Delivery {
		fmt.Fprintln(stderr, msg)
		return logx.Delivered
	})
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	cmd := args[0]
	var o options
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	fs.StringVar(&o.config, "config", "", "")
	fs.StringVar(&o.journalDir, "journal-dir", "", "")
	fs.StringVar(&o.server, "server", "", "")
	fs.UintVar(&o.port, "port", 443, "")
	fs.StringVar(&o.user, "user", "", "")
	fs.StringVar(&o.passwordFile, "password-file", "", "")
	fs.StringVar(&o.tp, "thumbprint", "", "")
	fs.StringVar(&o.out, "out", "", "")
	fs.Var(&o.libs, "lib", "")
	fs.DurationVar(&o.since, "since", 24*time.Hour, "")
	fs.IntVar(&o.logLines, "log-lines", 2000, "")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if o.logLines < 1 || o.since <= 0 {
		fmt.Fprintln(stderr, "openvddk-admin: -log-lines and -since must be positive")
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "openvddk-admin %s: unexpected argument %q\n", cmd, fs.Arg(0))
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var err error
	switch cmd {
	case "version":
		fmt.Fprintf(stdout, "openvddk-admin %s (%s); %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Compatibility)
		return 0
	case "journal":
		err = cmdJournal(o, stdout)
	case "attachments":
		err = withVCenter(ctx, o, func(w *worker) error { return cmdAttachments(ctx, w, stdout) })
	case "reconcile":
		err = withVCenter(ctx, o, func(w *worker) error { return cmdReconcile(ctx, w, stdout) })
	case "cleanup":
		left := false
		err = withVCenter(ctx, o, func(w *worker) error {
			var e error
			left, e = cmdCleanup(ctx, w, stdout)
			return e
		})
		if err == nil && left {
			return 3
		}
	case "bundle":
		err = cmdBundle(ctx, o, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "openvddk-admin: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "openvddk-admin %s: %v\n", cmd, err)
		return 1
	}
	return 0
}

// journalDirOf is the journal directory the library would use with the
// same configuration file.
func journalDirOf(o options) (string, *config.Config, error) {
	cfg := config.Empty()
	if o.config != "" {
		c, err := config.Load(o.config)
		if err != nil {
			return "", nil, fmt.Errorf("configuration file: %w", err)
		}
		cfg = c
	}
	if o.journalDir != "" {
		return o.journalDir, cfg, nil
	}
	if v, ok := cfg.Get("OpenVDDK.hotadd.journalDir"); ok && v != "" {
		return v, cfg, nil
	}
	return defaultJournalDir, cfg, nil
}

// openJournal opens an existing journal directory; unlike the library it
// never creates one.
func openJournal(dir string) (*hotadd.Journal, error) {
	if _, err := os.Lstat(dir); err != nil {
		return nil, err
	}
	j, err := hotadd.OpenJournal(dir)
	if err != nil && os.Geteuid() != 0 {
		return nil, fmt.Errorf("%w (the journal belongs to root; run as root)", err)
	}
	return j, err
}

// worker is a signed-in view of this worker VM.
type worker struct {
	sess    *vsphere.Session
	plat    vsphere.Platform
	host    hotadd.SystemHost
	journal *hotadd.Journal
	jdir    string
	proxy   hotadd.ProxyInfo
}

func withVCenter(ctx context.Context, o options, f func(*worker) error) error {
	if os.Geteuid() != 0 {
		return errors.New("must run as root (the journal, the BIOS UUID and the SCSI devices are root's)")
	}
	jdir, _, err := journalDirOf(o)
	if err != nil {
		return err
	}
	j, err := openJournal(jdir)
	if err != nil {
		return fmt.Errorf("journal %s: %w", jdir, err)
	}
	s, err := login(ctx, o)
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s.Logout(c)
		cancel()
	}()
	w := &worker{sess: s, plat: vsphere.Platform{S: s}, host: hotadd.SystemHost{H: linuxdev.System}, journal: j, jdir: jdir}
	if w.proxy, err = hotadd.IdentifyProxy(ctx, w.plat, w.host); err != nil {
		return fmt.Errorf("identifying this worker VM in vCenter: %w", err)
	}
	return f(w)
}

func login(ctx context.Context, o options) (*vsphere.Session, error) {
	if o.server == "" || o.user == "" || o.passwordFile == "" {
		return nil, errors.New("needs -server, -user and -password-file")
	}
	pw, err := readPassword(o.passwordFile)
	if err != nil {
		return nil, err
	}
	defer wipe(pw)
	s, err := vsphere.Login(ctx, vsphere.ConnectSpec{
		Server: o.server, Port: uint32(o.port), Thumbprint: o.tp, User: o.user, Password: pw,
		UserAgent: buildinfo.UserAgent(),
	})
	if err != nil {
		return nil, fmt.Errorf("signing in to %s: %w", o.server, err)
	}
	return s, nil
}

// readPassword returns the first line of the file, without its line end.
func readPassword(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("password file: %w", err)
	}
	defer f.Close()
	line, err := bufio.NewReader(f).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("password file: %w", err)
	}
	pw := bytes.TrimRight(line, "\r\n") // shares line's memory, so wiping pw wipes it
	if len(pw) == 0 {
		return nil, errors.New("password file: empty")
	}
	return pw, nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func busOf(p hotadd.ProxyInfo, controllerKey int32) int32 {
	for _, c := range p.Controllers {
		if c.Key == controllerKey {
			return c.Bus
		}
	}
	return -1
}
