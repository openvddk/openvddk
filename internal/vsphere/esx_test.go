// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/nfc"
)

// soapServer answers each call by its operation name with the body answer
// returns (a SOAP fault when answer returns ok false) and records the
// requests.
type soapServer struct {
	mu   sync.Mutex
	reqs []string
}

var opName = regexp.MustCompile(`<Body[^>]*>\s*<([A-Za-z_]+)`)

func (f *soapServer) session(t *testing.T, server string, esx bool, answer func(op, req string) (string, bool)) *Session {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req := strings.ReplaceAll(string(b), "soapenv:Body", "Body")
		m := opName.FindStringSubmatch(req)
		op := ""
		if m != nil {
			op = m[1]
		}
		f.mu.Lock()
		f.reqs = append(f.reqs, req)
		f.mu.Unlock()
		body, ok := answer(op, req)
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			body = `<soapenv:Fault><faultcode>ServerFaultCode</faultcode><faultstring>` + body + `</faultstring>` +
				`<detail><ManagedObjectNotFoundFault xmlns="urn:vim25" xsi:type="ManagedObjectNotFound"><obj type="NfcService">x</obj></ManagedObjectNotFoundFault></detail></soapenv:Fault>`
		}
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/" xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<soapenv:Body>`+body+`</soapenv:Body></soapenv:Envelope>`)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL + "/sdk")
	if err != nil {
		t.Fatal(err)
	}
	sc := soap.NewClient(u, true)
	return &Session{c: &vim25.Client{Client: sc, RoundTripper: sc}, server: server, esx: esx}
}

func (f *soapServer) count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if m := opName.FindStringSubmatch(r); m != nil && m[1] == op {
			n++
		}
	}
	return n
}

func (f *soapServer) last(op string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if m := opName.FindStringSubmatch(f.reqs[i]); m != nil && m[1] == op {
			return f.reqs[i]
		}
	}
	return ""
}

func internalContentAnswer(ref string) string {
	return `<RetrieveInternalContentResponse xmlns="urn:vim25"><returnval>` +
		`<agentManager type="AgentManager">x-agentmgr</agentManager>` +
		`<nfcService type="NfcService">` + ref + `</nfcService>` +
		`<proxyService type="ProxyService">x-proxy-service</proxyService>` +
		`</returnval></RetrieveInternalContentResponse>`
}

func ticketAnswer(host, thumb string) string {
	h := ""
	if host != "" {
		h = `<host>` + host + `</host>`
	}
	tp := ""
	if thumb != "" {
		tp = `<sslThumbprint>` + thumb + `</sslThumbprint>`
	}
	return `<NfcGetVmFilesResponse xmlns="urn:vim25"><returnval>` + h + `<port>902</port>` + tp +
		`<service>nfc</service><serviceVersion>1.1</serviceVersion><sessionId>s-1</sessionId></returnval></NfcGetVmFilesResponse>`
}

const testThumb = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD"

// Through vCenter the NFC service object is still "nfcService": the name
// RetrieveInternalContent gives, as before (NET-0002); the ticket's host is
// used as given.
func TestNfcTicketVCenter(t *testing.T) {
	var f soapServer
	s := f.session(t, "vc.example", false, func(op, req string) (string, bool) {
		switch op {
		case "RetrieveInternalContent":
			return internalContentAnswer("nfcService"), true
		case "NfcGetVmFiles":
			return ticketAnswer("esx01.example", testThumb), true
		}
		return "unexpected " + op, false
	})
	for i := 0; i < 2; i++ {
		tk, err := s.NfcTicket(context.Background(), "vm-21")
		if err != nil {
			t.Fatal(err)
		}
		if tk.Host != "esx01.example" || tk.Port != 902 || tk.SSLThumbprint != testThumb || tk.Service != "nfc" || tk.SessionID != "s-1" {
			t.Fatalf("ticket %+v", tk)
		}
	}
	if got := f.last("NfcGetVmFiles"); !strings.Contains(got, `<_this type="NfcService">nfcService</_this>`) ||
		!strings.Contains(got, `<vm type="VirtualMachine">vm-21</vm>`) {
		t.Fatalf("NfcGetVmFiles request: %s", got)
	}
	// The service object is asked for once per session.
	if n := f.count("RetrieveInternalContent"); n != 1 {
		t.Errorf("RetrieveInternalContent called %d times", n)
	}
}

// On ESXi the NFC service object is "ha-nfc-service" and the ticket names
// no host: the library connects to the server it signed in to, keeping the
// name the caller gave, and the ticket's thumbprint (NET-0005).
func TestNfcTicketESXi(t *testing.T) {
	var f soapServer
	s := f.session(t, "esxi-b.example", true, func(op, req string) (string, bool) {
		switch op {
		case "RetrieveInternalContent":
			return internalContentAnswer("ha-nfc-service"), true
		case "NfcGetVmFiles":
			return ticketAnswer("", testThumb), true
		}
		return "unexpected " + op, false
	})
	tk, err := s.NfcTicket(context.Background(), "212")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Host != "esxi-b.example" || tk.SSLThumbprint != testThumb || tk.Port != 902 {
		t.Fatalf("ticket %+v", tk)
	}
	if got := f.last("NfcGetVmFiles"); !strings.Contains(got, `<_this type="NfcService">ha-nfc-service</_this>`) {
		t.Fatalf("NfcGetVmFiles request: %s", got)
	}
	// An IPv6 literal is used without brackets (net.JoinHostPort adds them).
	s.server = "[fd00::157]"
	if tk, err = s.NfcTicket(context.Background(), "212"); err != nil || tk.Host != "fd00::157" {
		t.Fatalf("ticket %+v, %v", tk, err)
	}
}

// When RetrieveInternalContent fails, the name each server type is known to
// use is asked for, and the call is tried again for the next ticket.
func TestNfcServiceFallback(t *testing.T) {
	for _, c := range []struct {
		esx  bool
		want string
	}{{false, "nfcService"}, {true, "ha-nfc-service"}} {
		var f soapServer
		s := f.session(t, "server.example", c.esx, func(op, req string) (string, bool) {
			if op == "NfcGetVmFiles" {
				return ticketAnswer("", testThumb), true
			}
			return "no such method", false
		})
		if _, err := s.NfcTicket(context.Background(), "1"); err != nil {
			t.Fatal(err)
		}
		if got := f.last("NfcGetVmFiles"); !strings.Contains(got, `<_this type="NfcService">`+c.want+`</_this>`) {
			t.Fatalf("esx=%v: NfcGetVmFiles request: %s", c.esx, got)
		}
		s.NfcTicket(context.Background(), "1")
		if n := f.count("RetrieveInternalContent"); n != 2 {
			t.Errorf("esx=%v: RetrieveInternalContent called %d times, want 2", c.esx, n)
		}
	}
}

// The reported failure: the server does not know the NFC service object
// asked for. The error names the call, as before.
func TestNfcTicketUnknownServiceObject(t *testing.T) {
	var f soapServer
	s := f.session(t, "esx.example", true, func(op, req string) (string, bool) {
		if op == "RetrieveInternalContent" {
			return internalContentAnswer("ha-nfc-service"), true
		}
		return "The object has already been deleted or has not been completely created", false
	})
	_, err := s.NfcTicket(context.Background(), "1")
	if err == nil || !strings.HasPrefix(err.Error(), "NfcGetVmFiles: ") {
		t.Fatalf("got %v", err)
	}
}

// A ticket without a certificate thumbprint is refused before anything is
// sent to the host's NFC port: nfc.Dial needs the pin (no TLS without it).
func TestTicketWithoutThumbprintRefused(t *testing.T) {
	var f soapServer
	s := f.session(t, "127.0.0.1", true, func(op, req string) (string, bool) {
		switch op {
		case "RetrieveInternalContent":
			return internalContentAnswer("ha-nfc-service"), true
		case "NfcGetVmFiles":
			return ticketAnswer("", ""), true
		}
		return "unexpected " + op, false
	})
	tk, err := s.NfcTicket(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if tk.SSLThumbprint != "" || tk.Host != "127.0.0.1" {
		t.Fatalf("ticket %+v", tk)
	}
	// A listener that greets as the authentication service does, so the
	// refusal comes from the missing pin and not from the connection.
	ln := greetingListener(t)
	tk.Host, tk.Port = "127.0.0.1", ln
	_, err = nfc.Dial(context.Background(), tk.Host, tk, nfc.Options{})
	if !errors.Is(err, nfc.ErrTLS) || !strings.Contains(err.Error(), "thumbprint") {
		t.Fatalf("got %v", err)
	}
}

// greetingListener accepts connections on a local port and sends the
// authentication service's greeting line; it returns the port.
func greetingListener(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			io.WriteString(c, "220 VMware Authentication Daemon Version 1.10: SSL Required\r\n")
			go func() { io.Copy(io.Discard, c); c.Close() }()
		}
	}()
	return mustPort(t, &url.URL{Host: ln.Addr().String()})
}

// Login tells an ESXi host (API type HostAgent) from vCenter.
func TestLoginDetectsESXi(t *testing.T) {
	for _, c := range []struct {
		model *simulator.Model
		esx   bool
	}{{simulator.VPX(), false}, {simulator.ESX(), true}} {
		m := c.model
		if err := m.Create(); err != nil {
			t.Fatal(err)
		}
		m.Service.TLS = new(tls.Config)
		srv := m.Service.NewServer()
		sum := sha1.Sum(srv.Certificate().Raw)
		s, err := Login(context.Background(), ConnectSpec{
			Server: srv.URL.Hostname(), Port: uint32(mustPort(t, srv.URL)), Thumbprint: hexColon(sum[:]),
			User: "user", Password: []byte("pass"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if s.IsESXi() != c.esx {
			t.Errorf("%s: IsESXi %v, want %v", s.About(), s.IsESXi(), c.esx)
		}
		if c.esx && !strings.HasPrefix(s.VCenterUUID(), "esx:") && s.c.ServiceContent.About.InstanceUuid == "" {
			t.Errorf("ESXi session UUID %q", s.VCenterUUID())
		}
		// On ESXi the backup NFC address is looked up through the VM's own
		// host; without one selected the ticket's host is used.
		if c.esx {
			vms, err := simulatorVMs(context.Background(), s)
			if err != nil || len(vms) == 0 {
				t.Fatalf("simulator VMs: %v %v", vms, err)
			}
			tk := nfc.Ticket{Host: "esx.example"}
			if a := s.NfcAddress(context.Background(), tk, vms[0].Value); a != "esx.example" {
				t.Errorf("NfcAddress %q", a)
			}
		}
		s.Logout(context.Background())
		srv.Close()
		m.Remove()
	}
}

func simulatorVMs(ctx context.Context, s *Session) ([]types.ManagedObjectReference, error) {
	v, err := view.NewManager(s.c).CreateContainerView(ctx, s.c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, err
	}
	defer v.Destroy(ctx)
	return v.Find(ctx, []string{"VirtualMachine"}, nil)
}
