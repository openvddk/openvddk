// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

// A fault answer to every call, as vCenter gives it when the role lacks a
// privilege.
func faultSession(t *testing.T, fault string) *Session {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<soapenv:Envelope xmlns:soapenc="http://schemas.xmlsoap.org/soap/encoding/" xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<soapenv:Body><soapenv:Fault><faultcode>ServerFaultCode</faultcode><faultstring>Permission to perform this operation was denied.</faultstring>
<detail>`+fault+`</detail></soapenv:Fault></soapenv:Body></soapenv:Envelope>`)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL + "/sdk")
	if err != nil {
		t.Fatal(err)
	}
	sc := soap.NewClient(u, true)
	return &Session{c: &vim25.Client{Client: sc, RoundTripper: sc}}
}

// A refused NfcGetVmFiles names the missing privilege, and the message
// carries no word the log redaction hides ("ticket: …"); other failures name
// the call (D-0036, V1).
func TestNfcTicketNoPermission(t *testing.T) {
	s := faultSession(t, `<NoPermissionFault xmlns="urn:vim25" xsi:type="NoPermission"><object type="VirtualMachine">vm-102</object><privilegeId>VirtualMachine.Provisioning.GetVmFiles</privilegeId></NoPermissionFault>`)
	_, err := s.NfcTicket(context.Background(), "vm-102")
	if !errors.Is(err, ErrNoPermission) || !strings.Contains(err.Error(), "needs VirtualMachine.Provisioning.GetVmFiles on vm-102") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "ticket:") {
		t.Errorf("message would be redacted: %v", err)
	}

	s = faultSession(t, `<SystemErrorFault xmlns="urn:vim25" xsi:type="SystemError"><reason>boom</reason></SystemErrorFault>`)
	_, err = s.NfcTicket(context.Background(), "vm-102")
	if err == nil || errors.Is(err, ErrNoPermission) || !strings.HasPrefix(err.Error(), "NfcGetVmFiles: ") {
		t.Fatalf("got %v", err)
	}
}

// A refused QueryChangedDiskAreas names the missing privilege (D-0035, V1).
func TestAllocatedAreasNoPermission(t *testing.T) {
	s := faultSession(t, `<NoPermissionFault xmlns="urn:vim25" xsi:type="NoPermission"><object type="VirtualMachine">vm-102</object><privilegeId>VirtualMachine.Provisioning.DiskRandomRead</privilegeId></NoPermissionFault>`)
	_, err := s.AllocatedAreas(context.Background(), "vm-102", "", 2000, 1<<30)
	if !errors.Is(err, ErrNoPermission) || !strings.Contains(err.Error(), "needs VirtualMachine.Provisioning.DiskRandomRead on vm-102") {
		t.Fatalf("got %v", err)
	}
}
