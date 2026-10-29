// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openvddk/openvddk/internal/buildinfo"
	"github.com/openvddk/openvddk/internal/config"
	"github.com/openvddk/openvddk/internal/hotadd"
	"github.com/openvddk/openvddk/internal/linuxdev"
)

// referenceSHA256 identifies VMware's libvixDiskLib.so.8.0.3 of VDDK 8.0.3
// build 23950268, the reference this project is compatible with.
const referenceSHA256 = "afa97c6285c17f55da1ca71caa5816eaf55c539c11a75f9974ddca9725364312"

var defaultLibs = []string{
	"/opt/vmware-vix-disklib-distrib/lib64/libvixDiskLib.so.8.0.3",
	"/usr/lib/vmware-vix-disklib/lib64/libvixDiskLib.so.8.0.3",
	"/usr/local/lib/vmware-vix-disklib/lib64/libvixDiskLib.so.8.0.3",
	"/usr/lib64/vmware-vix-disklib/lib64/libvixDiskLib.so.8.0.3",
}

// bundle is the content of a diagnostic archive (CONTRACT.md §105).
type bundle struct {
	names []string
	files map[string]*bytes.Buffer
}

func newBundle() *bundle { return &bundle{files: map[string]*bytes.Buffer{}} }

func (b *bundle) file(name string) *bytes.Buffer {
	if f, ok := b.files[name]; ok {
		return f
	}
	f := &bytes.Buffer{}
	b.files[name] = f
	b.names = append(b.names, name)
	return f
}

// secretValue matches "key=value" and "key: value" pairs whose key names a
// secret; the value is replaced, the key kept so the reader sees what was
// removed.
var secretValue = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|cookie|vmware_soap_session|session[_-]?key|api[_-]?key|credential[s]?)("?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;&"']+)`)

// secretKey: configuration keys whose values are never collected.
var secretKey = regexp.MustCompile(`(?i)pass|secret|token|cookie|credential`)

// redact removes secrets from every file: values of secret-looking pairs,
// and every occurrence of the password used for this run. It returns how
// many replacements were made.
func (b *bundle) redact(password []byte) int {
	n := 0
	for _, name := range b.names {
		f := b.files[name]
		data := f.Bytes()
		data = secretValue.ReplaceAllFunc(data, func(m []byte) []byte {
			n++
			sub := secretValue.FindSubmatch(m)
			return append(append(append([]byte{}, sub[1]...), sub[2]...), "<redacted>"...)
		})
		if len(password) >= 3 {
			if c := bytes.Count(data, password); c > 0 {
				n += c
				data = bytes.ReplaceAll(data, password, []byte("<redacted>"))
			}
		}
		f.Reset()
		f.Write(data)
	}
	return n
}

func (b *bundle) writeTarGz(w io.Writer, mtime time.Time) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, name := range b.names {
		data := b.files[name].Bytes()
		if err := tw.WriteHeader(&tar.Header{Name: "openvddk-bundle/" + name, Mode: 0o600, Size: int64(len(data)), ModTime: mtime, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// env is the machine the bundle describes; tests point it at a synthetic
// tree.
type env struct {
	root string // "/" normally
	host linuxdev.Host
	run  func(ctx context.Context, name string, args ...string) ([]byte, error)
	now  time.Time
}

func systemEnv() env {
	return env{root: "/", host: linuxdev.System, now: time.Now(), run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	}}
}

func (e env) path(p string) string { return filepath.Join(e.root, p) }

func cmdBundle(ctx context.Context, o options, stdout, stderr io.Writer) error {
	e := systemEnv()
	b := newBundle()
	jdir, cfg, cfgErr := journalDirOf(o)
	if cfgErr != nil {
		jdir = defaultJournalDir
		cfg = config.Empty()
	}

	sum := b.file("summary.txt")
	fmt.Fprintf(sum, "OpenVDDK diagnostic bundle, collected %s by openvddk-admin %s (%s).\n",
		e.now.UTC().Format("2006-01-02 15:04:05 UTC"), buildinfo.Version, buildinfo.Commit)
	fmt.Fprintf(sum, "Compatibility: %s.\n", buildinfo.Compatibility)
	fmt.Fprintln(sum, "Passwords, session cookies and keys are not collected; values that look like secrets are replaced by <redacted>.")
	fmt.Fprintln(sum)

	collectSystem(ctx, e, b.file("system.txt"))
	copyFile(e, b.file("modules.txt"), "/proc/modules")
	collectDevices(e, b.file("devices.txt"))
	collectLibraries(e, o.libs, b.file("library.txt"))
	collectConfig(o, cfg, cfgErr, jdir, b.file("config.txt"))
	collectJournal(e, jdir, b.file("journal.txt"))

	var password []byte
	vs := b.file("vsphere.txt")
	switch {
	case o.server == "":
		fmt.Fprintln(vs, "vCenter was not asked (no -server): the worker VM's view in vCenter is not included.")
	case os.Geteuid() != 0:
		fmt.Fprintln(vs, "vCenter was not asked: identifying this worker needs root.")
	default:
		pw, err := readPassword(o.passwordFile)
		if err != nil {
			fmt.Fprintf(vs, "vCenter was not asked: %v\n", err)
			break
		}
		password = append([]byte{}, pw...)
		wipe(pw)
		collectVCenter(ctx, o, jdir, vs)
	}
	defer wipe(password)

	collectLogs(ctx, e, cfg, o.since, o.logLines, b)

	n := b.redact(password)
	fmt.Fprintf(sum, "Files: %s.\n", strings.Join(b.names, ", "))
	fmt.Fprintf(sum, "Replacements made while removing secrets: %d.\n", n)

	out := o.out
	if out == "" {
		out = "openvddk-bundle-" + e.now.UTC().Format("20060102-150405") + ".tar.gz"
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := b.writeTarGz(f, e.now); err != nil {
		f.Close()
		os.Remove(out)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote %s (%d files). Please check it before you send it.\n", out, len(b.names))
	return nil
}

func copyFile(e env, w io.Writer, p string) {
	data, err := os.ReadFile(e.path(p))
	if err != nil {
		fmt.Fprintf(w, "%s: unavailable (%v)\n", p, err)
		return
	}
	w.Write(data)
}

func collectSystem(ctx context.Context, e env, w io.Writer) {
	host, _ := os.Hostname()
	fmt.Fprintf(w, "Host name: %s\n", dash(host))
	fmt.Fprintf(w, "Effective user ID: %d\n", os.Geteuid())
	fmt.Fprintf(w, "CPUs: %d\n", runtime.NumCPU())
	if mem, err := os.ReadFile(e.path("/proc/meminfo")); err == nil {
		for _, line := range strings.Split(string(mem), "\n") {
			if strings.HasPrefix(line, "MemTotal:") || strings.HasPrefix(line, "MemAvailable:") {
				fmt.Fprintln(w, strings.Join(strings.Fields(line), " "))
			}
		}
	}
	if up, err := os.ReadFile(e.path("/proc/uptime")); err == nil {
		if f := strings.Fields(string(up)); len(f) > 0 {
			if s, err := strconv.ParseFloat(f[0], 64); err == nil {
				fmt.Fprintf(w, "Up for: %s\n", time.Duration(s)*time.Second)
			}
		}
	}
	fmt.Fprintf(w, "Boot ID: %s\n", dash(e.host.BootID()))
	if u, err := e.host.ProductUUID(); err == nil {
		vs, _ := linuxdev.SMBIOSToVSphereUUID(u)
		fmt.Fprintf(w, "BIOS UUID: %s (vSphere form %s)\n", u, dash(vs))
	} else {
		fmt.Fprintf(w, "BIOS UUID: unavailable (%v)\n", err)
	}
	p := e.host.LVMProtection(ctx, e.run)
	fmt.Fprintf(w, "LVM protection of attached disks: %v (%s)\n", p.Safe, p.Reason)
	// How the devices file names the worker's own disks matters at a restart
	// with a source disk attached (HOTADD-0027).
	if df, err := os.ReadFile(e.path("/etc/lvm/devices/system.devices")); err == nil {
		fmt.Fprintln(w, "LVM devices file /etc/lvm/devices/system.devices:")
		for _, line := range strings.Split(strings.TrimSpace(string(df)), "\n") {
			if !strings.HasPrefix(line, "#") {
				fmt.Fprintf(w, "  %s\n", line)
			}
		}
	}
	if out, err := e.run(ctx, "ldd", "--version"); err == nil {
		fmt.Fprintf(w, "C library: %s\n", strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]))
	}
	fmt.Fprintln(w, "\nKernel:")
	copyFile(e, w, "/proc/version")
	fmt.Fprintln(w, "\nOperating system (/etc/os-release):")
	copyFile(e, w, "/etc/os-release")
}

func collectDevices(e env, w io.Writer) {
	hosts, _ := filepath.Glob(filepath.Join(e.host.Sys, "class", "scsi_host", "host*"))
	byDriver := map[string][]int{}
	for _, h := range hosts {
		n, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(h), "host"))
		if err != nil {
			continue
		}
		name, _ := os.ReadFile(filepath.Join(h, "proc_name"))
		drv := strings.TrimSpace(string(name))
		byDriver[drv] = append(byDriver[drv], n)
	}
	var drivers []string
	for d := range byDriver {
		drivers = append(drivers, d)
	}
	sort.Strings(drivers)
	fmt.Fprintln(w, "SCSI hosts by driver:")
	for _, d := range drivers {
		ns := byDriver[d]
		sort.Ints(ns)
		var names []string
		for _, n := range ns {
			names = append(names, fmt.Sprintf("host%d", n))
		}
		fmt.Fprintf(w, "  %-12s %d: %s\n", dash(d), len(ns), strings.Join(names, " "))
	}
	buses := e.host.SCSIBuses()
	var hs []int
	for h := range buses {
		hs = append(hs, h)
	}
	sort.Ints(hs)
	fmt.Fprint(w, "vSphere buses of the SCSI hosts:")
	for _, h := range hs {
		fmt.Fprintf(w, " host%d=scsi%d", h, buses[h])
	}
	fmt.Fprintln(w)
	disks, err := e.host.SCSIDisks()
	if err != nil {
		fmt.Fprintf(w, "Disks: unavailable (%v)\n", err)
		return
	}
	mounted := e.host.MountedDevices()
	fmt.Fprintf(w, "\nSCSI disks: %d\n", len(disks))
	for _, d := range disks {
		m := ""
		if mounted[[2]uint32{d.Major, d.Minor}] {
			m = ", mounted"
		}
		fmt.Fprintf(w, "  %-6s %-10s %s  %s  %s %s  holders %v%s\n", d.Name, d.HCTL, dash(d.WWID), gib(int64(d.Sectors)*512),
			dash(d.Vendor), dash(d.Model), d.Holders, m)
	}
}

func fileSHA256(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func libraryKind(p, sum string) string {
	if sum == referenceSHA256 {
		return "VMware VDDK 8.0.3 build 23950268 (the reference)"
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "unreadable"
	}
	switch {
	case bytes.Contains(data, []byte("OpenVDDK/")):
		return "OpenVDDK (its version is in the first log line: \"OpenVDDK <version> (<commit>)\")"
	case bytes.Contains(data, []byte("VMware")):
		return "VMware VDDK, another build"
	}
	return "unknown"
}

// collectLibraries describes the library files on this worker and which
// running processes have one loaded.
func collectLibraries(e env, libs []string, w io.Writer) {
	paths := append([]string{}, libs...)
	if len(paths) == 0 {
		paths = append(paths, defaultLibs...)
	}
	loaded := map[string][]string{}
	maps, _ := filepath.Glob(e.path("/proc/[0-9]*/maps"))
	for _, m := range maps {
		f, err := os.Open(m)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			i := strings.Index(line, "/")
			if i < 0 || !strings.Contains(line, "libvixDiskLib") {
				continue
			}
			file := line[i:]
			if seen[file] {
				continue
			}
			seen[file] = true
			pid := filepath.Base(filepath.Dir(m))
			comm, _ := os.ReadFile(filepath.Join(filepath.Dir(m), "comm"))
			loaded[file] = append(loaded[file], fmt.Sprintf("%s (%s)", pid, strings.TrimSpace(string(comm))))
		}
		f.Close()
	}
	described := map[string]bool{}
	for _, p := range paths {
		real, err := filepath.EvalSymlinks(e.path(p))
		if err != nil {
			fmt.Fprintf(w, "%s: not present\n", p)
			continue
		}
		described[real] = true
		sum, size, err := fileSHA256(real)
		if err != nil {
			fmt.Fprintf(w, "%s: unreadable (%v)\n", p, err)
			continue
		}
		st, _ := os.Stat(real)
		fmt.Fprintf(w, "%s\n  file      %s, %d bytes, modified %s\n  SHA-256   %s\n  kind      %s\n", p, real, size, when(st.ModTime()), sum, libraryKind(real, sum))
	}
	var files []string
	for f := range loaded {
		files = append(files, f)
	}
	sort.Strings(files)
	fmt.Fprintf(w, "\nRunning processes with a VixDiskLib library loaded: %d file%s.\n", len(files), plural(len(files)))
	for _, f := range files {
		note := ""
		if strings.HasSuffix(f, " (deleted)") {
			note = "  (replaced on disk after the process loaded it; the process still runs the old file)"
		}
		fmt.Fprintf(w, "  %s%s\n    processes: %s\n", f, note, strings.Join(loaded[f], ", "))
	}
}

func collectConfig(o options, cfg *config.Config, cfgErr error, jdir string, w io.Writer) {
	fmt.Fprintf(w, "Journal directory: %s\n", jdir)
	switch {
	case o.config == "":
		fmt.Fprintln(w, "Configuration file: not given (-config); the library's defaults are shown in the README.")
		return
	case cfgErr != nil:
		fmt.Fprintf(w, "Configuration file %s: %v\n", o.config, cfgErr)
		return
	}
	fmt.Fprintf(w, "Configuration file %s:\n", o.config)
	for _, k := range cfg.Keys() {
		v, _ := cfg.Get(k)
		if secretKey.MatchString(k) {
			v = "<redacted>"
		}
		fmt.Fprintf(w, "  %s = %q\n", k, v)
	}
	for _, warn := range cfg.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warn)
	}
}

func collectJournal(e env, jdir string, w io.Writer) {
	st, err := os.Lstat(jdir)
	if err != nil {
		fmt.Fprintf(w, "Journal %s: %v\n", jdir, err)
		return
	}
	fmt.Fprintf(w, "Journal directory %s: mode %v\n", jdir, st.Mode())
	entries, _ := os.ReadDir(jdir)
	for _, d := range entries {
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(w, "  %v %8d %s %s\n", info.Mode(), info.Size(), when(info.ModTime()), d.Name())
		}
	}
	fmt.Fprintln(w)
	j, err := openJournal(jdir)
	if err != nil {
		fmt.Fprintf(w, "Records not read: %v\n", err)
		return
	}
	writeJournal(w, j, jdir, e.host.Proc, e.host.BootID())
}

func collectVCenter(ctx context.Context, o options, jdir string, w io.Writer) {
	err := withVCenter(ctx, o, func(wk *worker) error {
		fmt.Fprintf(w, "vCenter %s: %s (instance %s)\n", o.server, wk.sess.About(), wk.sess.VCenterUUID())
		if d, err := wk.plat.Describe(ctx, wk.proxy.VM); err == nil {
			fmt.Fprintf(w, "Worker VM %s \"%s\", %s, on host %s \"%s\": %s\n\n", wk.proxy.VM, d.Name, d.PowerState, d.Host, d.HostName, dash(d.HostProduct))
		} else {
			fmt.Fprintf(w, "Worker VM %s: details unavailable (%v)\n\n", wk.proxy.VM, err)
		}
		writeAttachments(w, wk.proxy, wk.journal, wk.host)
		fmt.Fprintln(w, "\nWhat recovery would do now (nothing was changed):")
		as, errs := hotadd.Assess(ctx, wk.plat, wk.host, hotadd.Options{Journal: wk.journal}, wk.proxy.VM)
		writeAssessment(w, as, errs)
		return nil
	})
	if err != nil {
		fmt.Fprintf(w, "vCenter %s: %v\n", o.server, err)
	}
}

// collectLogs gathers the library's own log files, the trace file named in
// the configuration, and the system journal's lines about the library and
// about SCSI devices.
func collectLogs(ctx context.Context, e env, cfg *config.Config, since time.Duration, lines int, b *bundle) {
	cutoff := e.now.Add(-since)
	var files []string
	for _, dir := range []string{"/tmp", "/var/tmp", os.TempDir()} {
		m, _ := filepath.Glob(filepath.Join(e.path(dir), "openvddk-*", "openvddk-*.log"))
		files = append(files, m...)
	}
	sort.Strings(files)
	files = dedupe(files)
	sort.Slice(files, func(a, c int) bool { return mtime(files[a]).After(mtime(files[c])) })
	kept := 0
	for _, f := range files {
		if kept == 5 || mtime(f).Before(cutoff) {
			break
		}
		if why := notLibraryLog(f); why != "" {
			fmt.Fprintf(b.file("logs/skipped.txt"), "%s: skipped (%s)\n", f, why)
			continue
		}
		tailFile(b.file("logs/library-"+filepath.Base(f)), f, lines, true)
		kept++
	}
	for _, key := range []string{"OpenVDDK.trace.file"} {
		if p, ok := cfg.Get(key); ok && p != "" {
			tailFile(b.file("logs/trace-"+filepath.Base(p)), p, lines, false)
		}
	}
	sinceArg := "--since=-" + strconv.Itoa(int(since.Seconds())) + "s"
	if out, err := e.run(ctx, "journalctl", "--no-pager", "-o", "short-iso", sinceArg, "-n", strconv.Itoa(lines), "-g", "OpenVDDK|VixDiskLib|vddk"); err == nil {
		b.file("logs/system-journal-library.txt").Write(out)
	}
	if out, err := e.run(ctx, "journalctl", "--no-pager", "-o", "short-iso", "-k", sinceArg, "-n", strconv.Itoa(lines), "-g", "scsi|sd[a-z]+|pvscsi|I/O error|blk_update"); err == nil {
		b.file("logs/kernel-scsi.txt").Write(out)
	}
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func mtime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// notLibraryLog says why p is not a log file the library wrote for one of
// the worker's users, or "" if it is: a regular file with one name, not a
// link, in a real directory that belongs to the file's owner and is closed
// to others, as the library creates it. The bundle runs as root and leaves
// the worker, so a link planted in /tmp must not bring another file into it.
func notLibraryLog(p string) string {
	fi, err := os.Lstat(p)
	if err != nil {
		return err.Error()
	}
	di, err := os.Lstat(filepath.Dir(p))
	if err != nil {
		return err.Error()
	}
	fst, ok1 := fi.Sys().(*syscall.Stat_t)
	dst, ok2 := di.Sys().(*syscall.Stat_t)
	switch {
	case !fi.Mode().IsRegular():
		return "not a regular file"
	case !di.IsDir():
		return "its directory is a link"
	case !ok1 || !ok2 || fst.Uid != dst.Uid:
		return "the file and its directory have different owners"
	case di.Mode().Perm()&0o077 != 0:
		return "its directory is open to other users"
	case fst.Nlink != 1:
		return "the file has other names (hard links)"
	}
	return ""
}

// tailFile keeps the last n lines of a file; with noFollow the path itself
// must not be a link.
func tailFile(w *bytes.Buffer, p string, n int, noFollow bool) {
	flags := os.O_RDONLY
	if noFollow {
		flags |= syscall.O_NOFOLLOW
	}
	f, err := os.OpenFile(p, flags, 0)
	if err != nil {
		fmt.Fprintf(w, "%s: unavailable (%v)\n", p, err)
		return
	}
	defer f.Close()
	if n < 1 {
		n = 1
	}
	ring := make([]string, 0, n)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	total := 0
	for sc.Scan() {
		total++
		if len(ring) == n {
			ring = ring[1:]
		}
		ring = append(ring, sc.Text())
	}
	fmt.Fprintf(w, "# %s: last %d of %d lines\n", p, len(ring), total)
	for _, l := range ring {
		fmt.Fprintln(w, l)
	}
}
