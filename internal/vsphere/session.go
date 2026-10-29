// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package vsphere is the vSphere control plane (CONTRACT.md sections 16,
// 20, 21): sessions, inventory resolution, proxy reconfiguration, access
// notifications and datastore file reads, built on govmomi.
package vsphere

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// ConnectSpec holds what is needed to open a session.
type ConnectSpec struct {
	Server     string
	Port       uint32
	Thumbprint string // SHA-1 or SHA-256 of the leaf certificate, hex, optional colons
	User       string
	Password   []byte
	UserAgent  string // client identification; govmomi's default when empty
}

// Session is an authenticated vCenter or ESXi session.
type Session struct {
	c      *vim25.Client
	sm     *session.Manager
	server string
	vcUUID string
	// esx: the server is an ESXi host signed in to directly (its API type is
	// HostAgent), not vCenter (D-0052).
	esx bool

	nfcMu  sync.Mutex
	nfcRef *types.ManagedObjectReference // the NFC service, once known
}

// apiTypeHost is ServiceContent.About.ApiType of an ESXi host; vCenter
// answers "VirtualCenter" (NET-0005).
const apiTypeHost = "HostAgent"

// ErrTLS reports a TLS identity failure (thumbprint mismatch or untrusted
// certificate). TLS is never downgraded or skipped silently (section 21).
var ErrTLS = errors.New("TLS server identity not verified")

// ErrAuth reports rejected credentials.
var ErrAuth = errors.New("authentication failed")

// Login opens a session.
func Login(ctx context.Context, spec ConnectSpec) (*Session, error) {
	host := spec.Server
	if spec.Port != 0 && spec.Port != 443 {
		host = net.JoinHostPort(spec.Server, strconv.Itoa(int(spec.Port)))
	} else if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]" // IPv6 literal
	}
	u := &url.URL{Scheme: "https", Host: host, Path: "/sdk"}
	cfg, err := tlsConfig(spec.Server, spec.Thumbprint)
	if err != nil {
		return nil, err
	}
	sc := soap.NewClient(u, false)
	if spec.UserAgent != "" {
		sc.UserAgent = spec.UserAgent
	}
	tr := sc.DefaultTransport()
	tr.TLSClientConfig = cfg
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}, Config: cfg}
	tr.DialTLSContext = dialer.DialContext
	tr.TLSHandshakeTimeout = 30 * time.Second

	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		sc.CloseIdleConnections()
		return nil, classifyTransport(err)
	}
	sm := session.NewManager(vc)
	if err := sm.Login(ctx, url.UserPassword(spec.User, string(spec.Password))); err != nil {
		sc.CloseIdleConnections()
		if isAuthFault(err) {
			return nil, fmt.Errorf("%w: %v", ErrAuth, err)
		}
		return nil, classifyTransport(err)
	}
	s := &Session{c: vc, sm: sm, server: spec.Server, esx: vc.ServiceContent.About.ApiType == apiTypeHost}
	if vc.ServiceContent.About.InstanceUuid != "" {
		s.vcUUID = vc.ServiceContent.About.InstanceUuid
	} else {
		s.vcUUID = "esx:" + spec.Server
	}
	return s, nil
}

// IsNotAuthenticated reports whether err says the session is no longer
// valid (it expired or was removed in vCenter).
func IsNotAuthenticated(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "NotAuthenticated") || strings.Contains(s, "session is not authenticated")
}

// Logout ends the session and closes its connections to the server. The
// client's connection pool would otherwise keep one HTTPS connection open
// for every session ever made in the process (T1, HOTADD-0030).
func (s *Session) Logout(ctx context.Context) error {
	if s == nil || s.sm == nil {
		return nil
	}
	err := s.sm.Logout(ctx)
	s.c.CloseIdleConnections()
	return err
}

// Client exposes the underlying client for packages of this module.
func (s *Session) Client() *vim25.Client { return s.c }

// Server is the server name the session was opened against.
func (s *Session) Server() string { return s.server }

// VCenterUUID identifies the vCenter instance ("esx:<server>" for a direct
// ESXi session).
func (s *Session) VCenterUUID() string { return s.vcUUID }

// IsESXi reports whether the session is with an ESXi host directly, not
// with vCenter (D-0052).
func (s *Session) IsESXi() bool { return s.esx }

// About describes the server.
func (s *Session) About() string {
	a := s.c.ServiceContent.About
	return fmt.Sprintf("%s %s build %s (API %s)", a.Name, a.Version, a.Build, a.ApiVersion)
}

func tlsConfig(server, thumbprint string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: strings.Trim(server, "[]")}
	if thumbprint == "" {
		return cfg, nil // standard verification against the system roots
	}
	want, err := parseThumbprint(thumbprint)
	if err != nil {
		return nil, err
	}
	// The pin replaces chain verification but is always enforced.
	cfg.InsecureSkipVerify = true
	cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
		if len(raw) == 0 {
			return fmt.Errorf("%w: no certificate presented", ErrTLS)
		}
		var got []byte
		if len(want) == sha1.Size {
			s := sha1.Sum(raw[0])
			got = s[:]
		} else {
			s := sha256.Sum256(raw[0])
			got = s[:]
		}
		if subtle.ConstantTimeCompare(got, want) != 1 {
			return fmt.Errorf("%w: certificate thumbprint does not match the pinned value", ErrTLS)
		}
		return nil
	}
	return cfg, nil
}

func parseThumbprint(s string) ([]byte, error) {
	h := strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(s))
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != sha1.Size && len(b) != sha256.Size) {
		return nil, fmt.Errorf("%w: malformed thumbprint", ErrTLS)
	}
	return b, nil
}

func classifyTransport(err error) error {
	var cv *tls.CertificateVerificationError
	var ui x509.UnknownAuthorityError
	var hn x509.HostnameError
	if errors.Is(err, ErrTLS) || errors.As(err, &cv) || errors.As(err, &ui) || errors.As(err, &hn) {
		if errors.Is(err, ErrTLS) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrTLS, err)
	}
	return err
}
