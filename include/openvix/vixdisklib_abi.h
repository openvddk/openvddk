/*
 * Copyright 2026 Andrija Panic and the OpenVDDK contributors
 * SPDX-License-Identifier: Apache-2.0
 */

/*
 * Independently authored declarations of the VixDiskLib C ABI, generation 8
 * (reference: VDDK 8.0.3 build 23950268, Linux x86-64, LP64).
 *
 * Only interface facts required for binary interoperability are reproduced:
 * identifiers, scalar widths, field order, numeric constants and calling
 * signatures. Every layout below is pinned by static assertions in
 * cabi/layout_assert.c and by abi/vddk-8.0.3/layout.json.
 *
 * Provenance: see PROVENANCE.md and docs/observations/ABI-0001.
 */
#ifndef OPENVIX_VIXDISKLIB_ABI_H
#define OPENVIX_VIXDISKLIB_ABI_H

#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ---- scalar types ------------------------------------------------------ */

typedef uint64_t VixError;             /* low 16 bits carry the code       */
typedef uint64_t VixDiskLibSectorType; /* sector index or sector count     */
typedef char     VixDiskLibBool;       /* one byte, 0 = false              */

#define VIX_OK                        ((VixError)0)
/* Returned by the *Async calls when the request was queued; the result is
 * delivered to the completion callback. */
#define VIX_ASYNC                     ((VixError)25000)
#define VIX_ERROR_CODE(err)           ((err) & 0xFFFF)

#define VIXDISKLIB_SECTOR_SIZE        512

/* Chunk limits for QueryAllocatedBlocks, in sectors. */
#define VIXDISKLIB_MIN_CHUNK_SIZE     128u
#define VIXDISKLIB_MAX_CHUNK_SIZE     131072u
#define VIXDISKLIB_MAX_CHUNK_NUMBER   524288u

/* Open flags. */
#define VIXDISKLIB_FLAG_OPEN_UNBUFFERED          0x0001u
#define VIXDISKLIB_FLAG_OPEN_SINGLE_LINK         0x0002u
#define VIXDISKLIB_FLAG_OPEN_READ_ONLY           0x0004u
#define VIXDISKLIB_FLAG_OPEN_COMPRESSION_ZLIB    0x0010u
#define VIXDISKLIB_FLAG_OPEN_COMPRESSION_FASTLZ  0x0020u
#define VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ   0x0040u
#define VIXDISKLIB_FLAG_OPEN_COMPRESSION_MASK    0x03F0u

/* ---- enumerations (all are 4-byte int in this ABI) ---------------------- */

typedef enum {
   VIXDISKLIB_DISK_MONOLITHIC_SPARSE = 1,
   VIXDISKLIB_DISK_MONOLITHIC_FLAT   = 2,
   VIXDISKLIB_DISK_SPLIT_SPARSE      = 3,
   VIXDISKLIB_DISK_SPLIT_FLAT        = 4,
   VIXDISKLIB_DISK_VMFS_FLAT         = 5,
   VIXDISKLIB_DISK_STREAM_OPTIMIZED  = 6,
   VIXDISKLIB_DISK_VMFS_THIN         = 7,
   VIXDISKLIB_DISK_VMFS_SPARSE       = 8,
   VIXDISKLIB_DISK_UNKNOWN           = 256
} VixDiskLibDiskType;

typedef enum {
   VIXDISKLIB_ADAPTER_IDE           = 1,
   VIXDISKLIB_ADAPTER_SCSI_BUSLOGIC = 2,
   VIXDISKLIB_ADAPTER_SCSI_LSILOGIC = 3,
   VIXDISKLIB_ADAPTER_UNKNOWN       = 256
} VixDiskLibAdapterType;

typedef enum {
   VIXDISKLIB_CRED_UID       = 1,
   VIXDISKLIB_CRED_SESSIONID = 2,
   VIXDISKLIB_CRED_TICKETID  = 3,
   VIXDISKLIB_CRED_SSPI      = 4,
   VIXDISKLIB_CRED_UNKNOWN   = 256
} VixDiskLibCredType;

typedef enum {
   VIXDISKLIB_SPEC_VMX             = 0,
   VIXDISKLIB_SPEC_VSTORAGE_OBJECT = 1,
   VIXDISKLIB_SPEC_DATASTORE       = 2,
   VIXDISKLIB_SPEC_UNKNOWN         = 256
} VixDiskLibSpecType;

/* ---- structures --------------------------------------------------------- */

typedef struct {
   uint32_t cylinders;
   uint32_t heads;
   uint32_t sectors;
} VixDiskLibGeometry;                                   /* 12 bytes */

typedef struct {
   VixDiskLibDiskType    diskType;
   VixDiskLibAdapterType adapterType;
   uint16_t              hwVersion;
   VixDiskLibSectorType  capacity;
   uint32_t              logicalSectorSize;
   uint32_t              physicalSectorSize;
} VixDiskLibCreateParams;                               /* 32 bytes */

typedef struct {
   char *id;
   char *datastoreMoRef;
   char *ssId;
} VixDiskLibVStorageObjectSpec;                         /* 24 bytes */

typedef struct {
   char *datastoreMoRef;
   char *diskFolder;
} VixDiskLibDatastoreSpec;                              /* 16 bytes */

/* Opaque; the field that points to it is reserved for library use. */
typedef struct VixDiskLibConnectParamsState VixDiskLibConnectParamsState;

/* Opaque; never dereferenced by this implementation's consumers. */
struct VixDiskLibTicketIdCreds;

typedef struct {
   char *vmxSpec;
   char *serverName;
   char *thumbPrint;
   long  privateUse;
   VixDiskLibCredType credType;
   union VixDiskLibCreds {
      struct VixDiskLibUidPasswdCreds {
         char *userName;
         char *password;
      } uid;
      struct VixDiskLibSessionIdCreds {
         char *cookie;
         char *userName;
         char *key;
      } sessionId;
      struct VixDiskLibTicketIdCreds *ticketId;
   } creds;
   uint32_t port;
   uint32_t nfcHostPort;
   char *vimApiVer;
   char  reserved[8];
   VixDiskLibConnectParamsState *state;
   union {
      VixDiskLibVStorageObjectSpec vStorageObjSpec;
      VixDiskLibDatastoreSpec      dsSpec;
   } spec;
   VixDiskLibSpecType specType;
} VixDiskLibConnectParams;                              /* 128 bytes */

typedef struct {
   VixDiskLibGeometry    biosGeo;
   VixDiskLibGeometry    physGeo;
   VixDiskLibSectorType  capacity;
   VixDiskLibAdapterType adapterType;
   int                   numLinks;
   char                 *parentFileNameHint;
   char                 *uuid;
   uint32_t              logicalSectorSize;
   uint32_t              physicalSectorSize;
} VixDiskLibInfo;                                       /* 64 bytes */

typedef struct {
   VixDiskLibSectorType offset;
   VixDiskLibSectorType length;
} VixDiskLibBlock;                                      /* 16 bytes */

typedef struct {
   uint32_t        numBlocks;
   VixDiskLibBlock blocks[1];                           /* variable length */
} VixDiskLibBlockList;                                  /* 24-byte header+1 */

/* ---- opaque handles ----------------------------------------------------- */

typedef struct VixDiskLibHandleStruct VixDiskLibHandleStruct;
typedef VixDiskLibHandleStruct *VixDiskLibHandle;
typedef struct VixDiskLibConnectParam *VixDiskLibConnection;

/* ---- callbacks ---------------------------------------------------------- */

typedef void (VixDiskLibGenericLogFunc)(const char *fmt, va_list args);
typedef VixDiskLibBool (*VixDiskLibProgressFunc)(void *progressData,
                                                 int percentCompleted);
typedef void (*VixDiskLibCompletionCB)(void *cbData, VixError result);

/* ---- functions (43 public entry points) --------------------------------- */

VixError VixDiskLib_InitEx(uint32_t majorVersion, uint32_t minorVersion,
                           VixDiskLibGenericLogFunc *log,
                           VixDiskLibGenericLogFunc *warn,
                           VixDiskLibGenericLogFunc *panic,
                           const char *libDir, const char *configFile);
VixError VixDiskLib_Init(uint32_t majorVersion, uint32_t minorVersion,
                         VixDiskLibGenericLogFunc *log,
                         VixDiskLibGenericLogFunc *warn,
                         VixDiskLibGenericLogFunc *panic,
                         const char *libDir);
void VixDiskLib_Exit(void);
const char *VixDiskLib_ListTransportModes(void);
VixError VixDiskLib_Cleanup(const VixDiskLibConnectParams *connectParams,
                            uint32_t *numCleanedUp, uint32_t *numRemaining);
VixError VixDiskLib_Connect(const VixDiskLibConnectParams *connectParams,
                            VixDiskLibConnection *connection);
VixError VixDiskLib_PrepareForAccess(const VixDiskLibConnectParams *connectParams,
                                     const char *identity);
VixError VixDiskLib_ConnectEx(const VixDiskLibConnectParams *connectParams,
                              VixDiskLibBool readOnly,
                              const char *snapshotRef,
                              const char *transportModes,
                              VixDiskLibConnection *connection);
VixError VixDiskLib_Disconnect(VixDiskLibConnection connection);
VixError VixDiskLib_EndAccess(const VixDiskLibConnectParams *connectParams,
                              const char *identity);
VixError VixDiskLib_Create(const VixDiskLibConnection connection,
                           const char *path,
                           const VixDiskLibCreateParams *createParams,
                           VixDiskLibProgressFunc progressFunc,
                           void *progressCallbackData);
VixError VixDiskLib_CreateChild(VixDiskLibHandle diskHandle,
                                const char *childPath,
                                VixDiskLibDiskType diskType,
                                VixDiskLibProgressFunc progressFunc,
                                void *progressCallbackData);
VixError VixDiskLib_Open(const VixDiskLibConnection connection,
                         const char *path, uint32_t flags,
                         VixDiskLibHandle *diskHandle);
VixError VixDiskLib_QueryAllocatedBlocks(VixDiskLibHandle diskHandle,
                                         VixDiskLibSectorType startSector,
                                         VixDiskLibSectorType numSectors,
                                         VixDiskLibSectorType chunkSize,
                                         VixDiskLibBlockList **blockList);
VixError VixDiskLib_FreeBlockList(VixDiskLibBlockList *blockList);
VixError VixDiskLib_GetInfo(VixDiskLibHandle diskHandle, VixDiskLibInfo **info);
void VixDiskLib_FreeInfo(VixDiskLibInfo *info);
const char *VixDiskLib_GetTransportMode(VixDiskLibHandle diskHandle);
VixError VixDiskLib_Close(VixDiskLibHandle diskHandle);
VixError VixDiskLib_Read(VixDiskLibHandle diskHandle,
                         VixDiskLibSectorType startSector,
                         VixDiskLibSectorType numSectors,
                         uint8_t *readBuffer);
VixError VixDiskLib_ReadAsync(VixDiskLibHandle diskHandle,
                              VixDiskLibSectorType startSector,
                              VixDiskLibSectorType numSectors,
                              uint8_t *readBuffer,
                              VixDiskLibCompletionCB callback, void *cbData);
VixError VixDiskLib_Write(VixDiskLibHandle diskHandle,
                          VixDiskLibSectorType startSector,
                          VixDiskLibSectorType numSectors,
                          const uint8_t *writeBuffer);
VixError VixDiskLib_WriteAsync(VixDiskLibHandle diskHandle,
                               VixDiskLibSectorType startSector,
                               VixDiskLibSectorType numSectors,
                               const uint8_t *writeBuffer,
                               VixDiskLibCompletionCB callback, void *cbData);
VixError VixDiskLib_Flush(VixDiskLibHandle diskHandle);
VixError VixDiskLib_Wait(VixDiskLibHandle diskHandle);
VixError VixDiskLib_ReadMetadata(VixDiskLibHandle diskHandle, const char *key,
                                 char *buf, size_t bufLen, size_t *requiredLen);
VixError VixDiskLib_WriteMetadata(VixDiskLibHandle diskHandle, const char *key,
                                  const char *val);
VixError VixDiskLib_GetMetadataKeys(VixDiskLibHandle diskHandle, char *keys,
                                    size_t maxLen, size_t *requiredLen);
VixError VixDiskLib_Unlink(VixDiskLibConnection connection, const char *path);
VixError VixDiskLib_Grow(VixDiskLibConnection connection, const char *path,
                         VixDiskLibSectorType capacity,
                         VixDiskLibBool updateGeometry,
                         VixDiskLibProgressFunc progressFunc,
                         void *progressCallbackData);
VixError VixDiskLib_Shrink(VixDiskLibHandle diskHandle,
                           VixDiskLibProgressFunc progressFunc,
                           void *progressCallbackData);
VixError VixDiskLib_Defragment(VixDiskLibHandle diskHandle,
                               VixDiskLibProgressFunc progressFunc,
                               void *progressCallbackData);
VixError VixDiskLib_Rename(const char *srcFileName, const char *dstFileName);
VixError VixDiskLib_Clone(const VixDiskLibConnection dstConnection,
                          const char *dstPath,
                          const VixDiskLibConnection srcConnection,
                          const char *srcPath,
                          const VixDiskLibCreateParams *vixCreateParams,
                          VixDiskLibProgressFunc progressFunc,
                          void *progressCallbackData,
                          VixDiskLibBool overWrite);
char *VixDiskLib_GetErrorText(VixError err, const char *locale);
void VixDiskLib_FreeErrorText(char *errMsg);
VixError VixDiskLib_IsAttachPossible(VixDiskLibHandle parent,
                                     VixDiskLibHandle child);
VixError VixDiskLib_Attach(VixDiskLibHandle parent, VixDiskLibHandle child);
VixError VixDiskLib_SpaceNeededForClone(VixDiskLibHandle diskHandle,
                                        VixDiskLibDiskType cloneDiskType,
                                        uint64_t *spaceNeeded);
VixError VixDiskLib_CheckRepair(const VixDiskLibConnection connection,
                                const char *filename, VixDiskLibBool repair);
VixError VixDiskLib_GetConnectParams(const VixDiskLibConnection connection,
                                     VixDiskLibConnectParams **connectParams);
void VixDiskLib_FreeConnectParams(VixDiskLibConnectParams *connectParams);
VixDiskLibConnectParams *VixDiskLib_AllocateConnectParams(void);

#ifdef __cplusplus
}
#endif

#endif /* OPENVIX_VIXDISKLIB_ABI_H */
