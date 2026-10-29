// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"

	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// AuthorizationManager.DisableMethods / EnableMethods are vCenter methods
// outside the public WSDL, so govmomi's generated types do not include
// them. VMware's open-source govmomi publishes their request format in
// object/authorization_manager_internal.go (Apache-2.0); the types below
// follow that format, in the urn:vim25 namespace. What the calls do, and
// the MethodAlreadyDisabledFault on a second disable, are recorded in
// HOTADD-0002; where each fact comes from, in NET-0001.

type disabledMethodRequest struct {
	Method   string `xml:"method"`
	ReasonID string `xml:"reasonId,omitempty"`
}

type disableMethodsRequest struct {
	This     types.ManagedObjectReference   `xml:"_this"`
	Entity   []types.ManagedObjectReference `xml:"entity"`
	Method   []disabledMethodRequest        `xml:"method"`
	SourceID string                         `xml:"sourceId,omitempty"`
}

type disableMethodsBody struct {
	Req    *disableMethodsRequest `xml:"urn:vim25 DisableMethods,omitempty"`
	Res    *struct{}              `xml:"DisableMethodsResponse,omitempty"`
	Fault_ *soap.Fault            `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *disableMethodsBody) Fault() *soap.Fault { return b.Fault_ }

type enableMethodsRequest struct {
	This     types.ManagedObjectReference   `xml:"_this"`
	Entity   []types.ManagedObjectReference `xml:"entity"`
	Method   []string                       `xml:"method"`
	SourceID string                         `xml:"sourceId,omitempty"`
}

type enableMethodsBody struct {
	Req    *enableMethodsRequest `xml:"urn:vim25 EnableMethods,omitempty"`
	Res    *struct{}             `xml:"EnableMethodsResponse,omitempty"`
	Fault_ *soap.Fault           `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault,omitempty"`
}

func (b *enableMethodsBody) Fault() *soap.Fault { return b.Fault_ }

func disableMethods(ctx context.Context, r soap.RoundTripper, this types.ManagedObjectReference,
	entity []types.ManagedObjectReference, methods []string, source string) error {
	req := &disableMethodsRequest{This: this, Entity: entity, SourceID: source}
	for _, m := range methods {
		req.Method = append(req.Method, disabledMethodRequest{Method: m})
	}
	var reqBody, resBody disableMethodsBody
	reqBody.Req = req
	return r.RoundTrip(ctx, &reqBody, &resBody)
}

func enableMethods(ctx context.Context, r soap.RoundTripper, this types.ManagedObjectReference,
	entity []types.ManagedObjectReference, methods []string, source string) error {
	var reqBody, resBody enableMethodsBody
	reqBody.Req = &enableMethodsRequest{This: this, Entity: entity, Method: methods, SourceID: source}
	return r.RoundTrip(ctx, &reqBody, &resBody)
}
