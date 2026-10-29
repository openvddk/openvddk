/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Prints the ABI layout as JSON (CONTRACT.md section 10).
 *
 * Built against this project's declarations in public CI. In the private
 * conformance environment it can also be built against the official
 * declarations (-DOVX_OFFICIAL_HEADER -I<ref>/include); both outputs must
 * equal abi/vddk-8.0.3/layout.json. Only numbers are recorded.
 */
#include <stddef.h>
#include <stdio.h>

#ifdef OVX_OFFICIAL_HEADER
#include "vixDiskLib.h"
typedef Bool probe_bool;
#else
#include "openvix/vixdisklib_abi.h"
typedef VixDiskLibBool probe_bool;
#endif

static int first = 1;

static void entry(const char *name, size_t value)
{
   printf("%s\n  \"%s\": %zu", first ? "{" : ",", name, value);
   first = 0;
}

#define SIZE(t)    entry("sizeof(" #t ")", sizeof(t))
#define OFF(t, f)  entry("offsetof(" #t "," #f ")", offsetof(t, f))

int main(void)
{
   SIZE(VixError);
   SIZE(VixDiskLibSectorType);
   SIZE(probe_bool);
   SIZE(VixDiskLibDiskType);
   SIZE(VixDiskLibAdapterType);
   SIZE(VixDiskLibCredType);
   SIZE(VixDiskLibSpecType);

   SIZE(VixDiskLibGeometry);
   OFF(VixDiskLibGeometry, cylinders);
   OFF(VixDiskLibGeometry, heads);
   OFF(VixDiskLibGeometry, sectors);

   SIZE(VixDiskLibCreateParams);
   OFF(VixDiskLibCreateParams, diskType);
   OFF(VixDiskLibCreateParams, adapterType);
   OFF(VixDiskLibCreateParams, hwVersion);
   OFF(VixDiskLibCreateParams, capacity);
   OFF(VixDiskLibCreateParams, logicalSectorSize);
   OFF(VixDiskLibCreateParams, physicalSectorSize);

   SIZE(VixDiskLibVStorageObjectSpec);
#ifndef OVX_NO_DATASTORE_SPEC /* added in VDDK 8; absent from 7.0.3 */
   SIZE(VixDiskLibDatastoreSpec);
#endif

   SIZE(VixDiskLibConnectParams);
   OFF(VixDiskLibConnectParams, vmxSpec);
   OFF(VixDiskLibConnectParams, serverName);
   OFF(VixDiskLibConnectParams, thumbPrint);
   OFF(VixDiskLibConnectParams, privateUse);
   OFF(VixDiskLibConnectParams, credType);
   OFF(VixDiskLibConnectParams, creds.uid.userName);
   OFF(VixDiskLibConnectParams, creds.uid.password);
   OFF(VixDiskLibConnectParams, creds.sessionId.cookie);
   OFF(VixDiskLibConnectParams, creds.sessionId.userName);
   OFF(VixDiskLibConnectParams, creds.sessionId.key);
   OFF(VixDiskLibConnectParams, port);
   OFF(VixDiskLibConnectParams, nfcHostPort);
   OFF(VixDiskLibConnectParams, vimApiVer);
   OFF(VixDiskLibConnectParams, reserved);
   OFF(VixDiskLibConnectParams, state);
   OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.id);
   OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.datastoreMoRef);
   OFF(VixDiskLibConnectParams, spec.vStorageObjSpec.ssId);
#ifndef OVX_NO_DATASTORE_SPEC
   OFF(VixDiskLibConnectParams, spec.dsSpec.datastoreMoRef);
   OFF(VixDiskLibConnectParams, spec.dsSpec.diskFolder);
#endif
   OFF(VixDiskLibConnectParams, specType);

   SIZE(VixDiskLibInfo);
   OFF(VixDiskLibInfo, biosGeo);
   OFF(VixDiskLibInfo, physGeo);
   OFF(VixDiskLibInfo, capacity);
   OFF(VixDiskLibInfo, adapterType);
   OFF(VixDiskLibInfo, numLinks);
   OFF(VixDiskLibInfo, parentFileNameHint);
   OFF(VixDiskLibInfo, uuid);
   OFF(VixDiskLibInfo, logicalSectorSize);
   OFF(VixDiskLibInfo, physicalSectorSize);

   SIZE(VixDiskLibBlock);
   OFF(VixDiskLibBlock, offset);
   OFF(VixDiskLibBlock, length);

   SIZE(VixDiskLibBlockList);
   OFF(VixDiskLibBlockList, numBlocks);
   OFF(VixDiskLibBlockList, blocks);

   SIZE(VixDiskLibHandle);
   SIZE(VixDiskLibConnection);

   printf("\n}\n");
   return 0;
}
