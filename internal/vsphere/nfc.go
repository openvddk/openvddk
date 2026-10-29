// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/openvddk/openvddk/internal/nfc"
)

// The NFC service object and its NfcGetVmFiles method are outside the
// public WSDL. Everything below was read from our own recordings of the
// reference's calls: through vCenter (NET-0002) and directly to ESXi 6.7
// and 7.0 (NET-0005). The reference asks ServiceInstance's
// RetrieveInternalContent for the NFC service object ("nfcService" on
// vCenter, "ha-nfc-service" on ESXi), then calls NfcGetVmFiles on it with
// the VM; the answer is a HostServiceTicket.

type retrieveInternalContentRequest struct {
	This types.ManagedObjectReference `xml:"_this"`
}

// internalContent is the part of RetrieveInternalContent's answer the
// library uses.
type internalContent struct {
	NfcService *types.ManagedObjectReference `xml:"nfcService,omitempty"`
}

type retrieveInternalContentResponse struct {
	Returnval internalContent `xml:"returnval"`
}

type retrieveInternalContentBody struct {
	Req    *retrieveInternalContentRequest  `xml:"urn:vim25 RetrieveInternalContent,omitempty"`
	Res    *retrieveInternalContentResponse `xml:"RetrieveInternalContentResponse,omitempty"`
	Fault_ *soap.Fault                      `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *retrieveInternalContentBody) Fault() *soap.Fault { return b.Fault_ }

type nfcGetVmFilesRequest struct {
	This types.ManagedObjectReference `xml:"_this"`
	VM   types.ManagedObjectReference `xml:"vm"`
}

type nfcGetVmFilesResponse struct {
	Returnval types.HostServiceTicket `xml:"returnval"`
}

type nfcGetVmFilesBody struct {
	Req    *nfcGetVmFilesRequest  `xml:"urn:vim25 NfcGetVmFiles,omitempty"`
	Res    *nfcGetVmFilesResponse `xml:"NfcGetVmFilesResponse,omitempty"`
	Fault_ *soap.Fault            `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *nfcGetVmFilesBody) Fault() *soap.Fault { return b.Fault_ }

// The NFC service objects as vCenter and ESXi name them (NET-0002,
// NET-0005); used when RetrieveInternalContent gives no answer.
const (
	nfcServiceVCenter = "nfcService"
	nfcServiceESXi    = "ha-nfc-service"
)

// nfcService returns the server's NFC service object, from
// RetrieveInternalContent as the reference finds it, remembered for the
// session. When that call fails or names none, the name the server type is
// known to use is returned (not remembered, so the next ticket asks again).
func (s *Session) nfcService(ctx context.Context) types.ManagedObjectReference {
	s.nfcMu.Lock()
	defer s.nfcMu.Unlock()
	if s.nfcRef != nil {
		return *s.nfcRef
	}
	var req, res retrieveInternalContentBody
	req.Req = &retrieveInternalContentRequest{This: vim25.ServiceInstance}
	if err := s.c.RoundTrip(ctx, &req, &res); err == nil && res.Res != nil &&
		res.Res.Returnval.NfcService != nil && res.Res.Returnval.NfcService.Value != "" {
		ref := *res.Res.Returnval.NfcService
		if ref.Type == "" {
			ref.Type = "NfcService"
		}
		s.nfcRef = &ref
		return ref
	}
	if s.esx {
		return types.ManagedObjectReference{Type: "NfcService", Value: nfcServiceESXi}
	}
	return types.ManagedObjectReference{Type: "NfcService", Value: nfcServiceVCenter}
}

// NfcTicket asks the server (vCenter, or an ESXi host directly) for a
// one-time NFC ticket for the VM's files. An ESXi host's ticket names no
// host: like the reference, the library then connects to the server the
// session was opened with (NET-0005). The ticket's certificate thumbprint
// is kept as it is; nfc.Dial refuses a ticket without one.
func (s *Session) NfcTicket(ctx context.Context, vm string) (nfc.Ticket, error) {
	var req, res nfcGetVmFilesBody
	req.Req = &nfcGetVmFilesRequest{This: s.nfcService(ctx), VM: vmRef(vm)}
	if err := s.c.RoundTrip(ctx, &req, &res); err != nil {
		if perr := withPermission(err, "NfcGetVmFiles"); perr != err {
			return nfc.Ticket{}, perr
		}
		return nfc.Ticket{}, fmt.Errorf("NfcGetVmFiles: %w", err)
	}
	if res.Res == nil {
		return nfc.Ticket{}, errors.New("the server returned no NFC ticket")
	}
	t := res.Res.Returnval
	host := t.Host
	if host == "" {
		host = strings.Trim(s.server, "[]")
	}
	return nfc.Ticket{
		Host:          host,
		Port:          int(t.Port),
		SSLThumbprint: t.SslThumbprint,
		Service:       t.Service,
		SessionID:     t.SessionId,
	}, nil
}

// NfcAddress returns the address to reach the ticket's host: the address of
// the host's vSphere Backup NFC interface when one is selected (the
// reference looks it up the same way, NET-0002, NET-0005), else the
// ticket's own. Through vCenter the host is found by the ticket's host
// name; on a direct ESXi session it is the host of the VM vm.
func (s *Session) NfcAddress(ctx context.Context, t nfc.Ticket, vm string) string {
	var host types.ManagedObjectReference
	if s.esx {
		var v mo.VirtualMachine
		if err := property.DefaultCollector(s.c).RetrieveOne(ctx, vmRef(vm), []string{"runtime.host"}, &v); err != nil ||
			v.Runtime.Host == nil {
			return t.Host
		}
		host = *v.Runtime.Host
	} else {
		found, err := methods.FindAllByDnsName(ctx, s.c, &types.FindAllByDnsName{
			This:    *s.c.ServiceContent.SearchIndex,
			DnsName: t.Host,
		})
		if err != nil || len(found.Returnval) == 0 {
			return t.Host
		}
		host = found.Returnval[0]
	}
	if a := s.backupNFCAddress(ctx, host); a != "" {
		return a
	}
	return t.Host
}

// backupNFCAddress returns the address of the host's selected vSphere
// Backup NFC interface, "" when none is selected or it cannot be read.
func (s *Session) backupNFCAddress(ctx context.Context, ref types.ManagedObjectReference) string {
	var h mo.HostSystem
	if err := property.DefaultCollector(s.c).RetrieveOne(ctx, ref,
		[]string{"configManager.virtualNicManager"}, &h); err != nil ||
		h.ConfigManager.VirtualNicManager == nil {
		return ""
	}
	nc, err := methods.QueryNetConfig(ctx, s.c, &types.QueryNetConfig{
		This:    *h.ConfigManager.VirtualNicManager,
		NicType: string(types.HostVirtualNicManagerNicTypeVSphereBackupNFC),
	})
	if err != nil || nc.Returnval == nil {
		return ""
	}
	for _, key := range nc.Returnval.SelectedVnic {
		for _, v := range nc.Returnval.CandidateVnic {
			if v.Key == key && v.Spec.Ip != nil && v.Spec.Ip.IpAddress != "" {
				return v.Spec.Ip.IpAddress
			}
		}
	}
	return ""
}
