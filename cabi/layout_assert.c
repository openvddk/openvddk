/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Compile-time pins for every structure, enumeration and scalar that crosses
 * the ABI (CONTRACT.md section 10). Values are the reference layout recorded
 * in abi/vddk-8.0.3/layout.json; tests/abi/layout_probe.c checks the same
 * values at run time and against the official declarations in private runs.
 */
#include <stddef.h>

#include "openvix/vixdisklib_abi.h"

#define PIN_SIZE(t, n)      _Static_assert(sizeof(t) == (n), #t " size")
#define PIN_OFF(t, f, n)    _Static_assert(offsetof(t, f) == (n), #t "." #f)

PIN_SIZE(VixError, 8);
PIN_SIZE(VixDiskLibSectorType, 8);
PIN_SIZE(VixDiskLibBool, 1);
PIN_SIZE(VixDiskLibDiskType, 4);
PIN_SIZE(VixDiskLibAdapterType, 4);
PIN_SIZE(VixDiskLibCredType, 4);
PIN_SIZE(VixDiskLibSpecType, 4);
PIN_SIZE(long, 8);
PIN_SIZE(void *, 8);

PIN_SIZE(VixDiskLibGeometry, 12);
PIN_OFF(VixDiskLibGeometry, cylinders, 0);
PIN_OFF(VixDiskLibGeometry, heads, 4);
PIN_OFF(VixDiskLibGeometry, sectors, 8);

PIN_SIZE(VixDiskLibCreateParams, 32);
PIN_OFF(VixDiskLibCreateParams, diskType, 0);
PIN_OFF(VixDiskLibCreateParams, adapterType, 4);
PIN_OFF(VixDiskLibCreateParams, hwVersion, 8);
PIN_OFF(VixDiskLibCreateParams, capacity, 16);
PIN_OFF(VixDiskLibCreateParams, logicalSectorSize, 24);
PIN_OFF(VixDiskLibCreateParams, physicalSectorSize, 28);

PIN_SIZE(VixDiskLibVStorageObjectSpec, 24);
PIN_SIZE(VixDiskLibDatastoreSpec, 16);

PIN_SIZE(VixDiskLibConnectParams, 128);
PIN_OFF(VixDiskLibConnectParams, vmxSpec, 0);
PIN_OFF(VixDiskLibConnectParams, serverName, 8);
PIN_OFF(VixDiskLibConnectParams, thumbPrint, 16);
PIN_OFF(VixDiskLibConnectParams, privateUse, 24);
PIN_OFF(VixDiskLibConnectParams, credType, 32);
PIN_OFF(VixDiskLibConnectParams, creds, 40);
PIN_OFF(VixDiskLibConnectParams, creds.uid.userName, 40);
PIN_OFF(VixDiskLibConnectParams, creds.uid.password, 48);
PIN_OFF(VixDiskLibConnectParams, creds.sessionId.cookie, 40);
PIN_OFF(VixDiskLibConnectParams, creds.sessionId.userName, 48);
PIN_OFF(VixDiskLibConnectParams, creds.sessionId.key, 56);
PIN_OFF(VixDiskLibConnectParams, port, 64);
PIN_OFF(VixDiskLibConnectParams, nfcHostPort, 68);
PIN_OFF(VixDiskLibConnectParams, vimApiVer, 72);
PIN_OFF(VixDiskLibConnectParams, reserved, 80);
PIN_OFF(VixDiskLibConnectParams, state, 88);
PIN_OFF(VixDiskLibConnectParams, spec, 96);
PIN_OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.id, 96);
PIN_OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.datastoreMoRef, 104);
PIN_OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.ssId, 112);
PIN_OFF(VixDiskLibConnectParams, spec.dsSpec.datastoreMoRef, 96);
PIN_OFF(VixDiskLibConnectParams, spec.dsSpec.diskFolder, 104);
PIN_OFF(VixDiskLibConnectParams, specType, 120);

PIN_SIZE(VixDiskLibInfo, 64);
PIN_OFF(VixDiskLibInfo, biosGeo, 0);
PIN_OFF(VixDiskLibInfo, physGeo, 12);
PIN_OFF(VixDiskLibInfo, capacity, 24);
PIN_OFF(VixDiskLibInfo, adapterType, 32);
PIN_OFF(VixDiskLibInfo, numLinks, 36);
PIN_OFF(VixDiskLibInfo, parentFileNameHint, 40);
PIN_OFF(VixDiskLibInfo, uuid, 48);
PIN_OFF(VixDiskLibInfo, logicalSectorSize, 56);
PIN_OFF(VixDiskLibInfo, physicalSectorSize, 60);

PIN_SIZE(VixDiskLibBlock, 16);
PIN_OFF(VixDiskLibBlock, offset, 0);
PIN_OFF(VixDiskLibBlock, length, 8);

PIN_SIZE(VixDiskLibBlockList, 24);
PIN_OFF(VixDiskLibBlockList, numBlocks, 0);
PIN_OFF(VixDiskLibBlockList, blocks, 8);
