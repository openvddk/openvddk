// Copyright 2026 Andrija Panic and the OpenVDDK contributors
// SPDX-License-Identifier: Apache-2.0

// Package vix defines the Vix error model used at the compatibility
// boundary: numeric codes (interface facts of the VixDiskLib ABI), their
// symbolic names, and this project's own descriptive text.
package vix

// Code is the low 16 bits of a VixError.
type Code uint16

// Codes returned by this implementation. The full catalog, including codes
// only ever produced by other implementations, is in catalog below.
const (
	OK                    Code = 0
	EFail                 Code = 1
	EOutOfMemory          Code = 2
	EInvalidArg           Code = 3
	EFileNotFound         Code = 4
	EObjectIsBusy         Code = 5
	ENotSupported         Code = 6
	EFileError            Code = 7
	ECancelled            Code = 10
	EFileReadOnly         Code = 11
	EFileAccessError      Code = 13
	EBufferTooSmall       Code = 24
	EObjectNotFound       Code = 25
	EHostNotConnected     Code = 26
	EAuthenticationFail   Code = 35
	EHostConnectionLost   Code = 36
	EInvalidHandle        Code = 1000
	EHostUserPermissions  Code = 3014
	EVMNotFound           Code = 4000
	ESnapshotNotFound     Code = 13003
	EHostServerNotFound   Code = 14008
	EHostConnRefused      Code = 14009
	EHostTCPSocketError   Code = 14010
	EHostTCPConnLost      Code = 14011
	EDiskInval            Code = 16000
	EDiskNoInit           Code = 16001
	EDiskNoIO             Code = 16002
	EDiskOutOfRange       Code = 16007
	EDiskKeyNotFound      Code = 16052
	EDiskSubsystemInit    Code = 16053
	EDiskInvalidConn      Code = 16054
	EDiskNoDevice         Code = 16065
	EDiskCapacityMismatch Code = 16067
	Async                 Code = 25000
	EAsyncMixedMode       Code = 26000
	ENetHTTPSSLConnect    Code = 30035
	ENetHTTPSSLSecurity   Code = 30201
)

type entry struct {
	name string
	text string
}

// catalog maps every code defined by the VixDiskLib 8 ABI to its symbolic
// name and a description written for this project.
var catalog = map[Code]entry{
	0:     {"VIX_OK", "The operation completed successfully"},
	1:     {"VIX_E_FAIL", "Unspecified failure"},
	2:     {"VIX_E_OUT_OF_MEMORY", "Memory allocation failed"},
	3:     {"VIX_E_INVALID_ARG", "An argument is not valid"},
	4:     {"VIX_E_FILE_NOT_FOUND", "The file does not exist"},
	5:     {"VIX_E_OBJECT_IS_BUSY", "The object is in use"},
	6:     {"VIX_E_NOT_SUPPORTED", "This operation is not supported here"},
	7:     {"VIX_E_FILE_ERROR", "A file access error occurred"},
	8:     {"VIX_E_DISK_FULL", "The destination has no free space"},
	9:     {"VIX_E_INCORRECT_FILE_TYPE", "The file is not of the expected type"},
	10:    {"VIX_E_CANCELLED", "The operation was cancelled"},
	11:    {"VIX_E_FILE_READ_ONLY", "The file or disk is read-only"},
	12:    {"VIX_E_FILE_ALREADY_EXISTS", "A file with that name exists already"},
	13:    {"VIX_E_FILE_ACCESS_ERROR", "Access to the file was denied"},
	14:    {"VIX_E_REQUIRES_LARGE_FILES", "The file system cannot hold files this large"},
	15:    {"VIX_E_FILE_ALREADY_LOCKED", "The file is locked by another process"},
	16:    {"VIX_E_VMDB", "A configuration database error occurred"},
	20:    {"VIX_E_NOT_SUPPORTED_ON_REMOTE_OBJECT", "The operation is not supported on a remote object"},
	21:    {"VIX_E_FILE_TOO_BIG", "The file is too large"},
	22:    {"VIX_E_FILE_NAME_INVALID", "The file name is invalid"},
	23:    {"VIX_E_ALREADY_EXISTS", "The object already exists"},
	24:    {"VIX_E_BUFFER_TOOSMALL", "The supplied buffer is too small"},
	25:    {"VIX_E_OBJECT_NOT_FOUND", "The object was not found"},
	26:    {"VIX_E_HOST_NOT_CONNECTED", "Not connected to the host"},
	27:    {"VIX_E_INVALID_UTF8_STRING", "The string is not valid UTF-8"},
	29:    {"VIX_E_UNFINISHED_JOB", "An asynchronous job has not finished"},
	30:    {"VIX_E_NEED_KEY", "An encryption key is required"},
	31:    {"VIX_E_OPERATION_ALREADY_IN_PROGRESS", "The same operation is already running"},
	32:    {"VIX_E_LICENSE", "A licensing error occurred"},
	34:    {"VIX_E_VM_HOST_DISCONNECTED", "The host of the virtual machine is disconnected"},
	35:    {"VIX_E_AUTHENTICATION_FAIL", "Authentication failed"},
	36:    {"VIX_E_HOST_CONNECTION_LOST", "Connection to the host lost"},
	41:    {"VIX_E_DUPLICATE_NAME", "The name is already in use"},
	44:    {"VIX_E_ARGUMENT_TOO_BIG", "An argument exceeds the allowed size"},
	1000:  {"VIX_E_INVALID_HANDLE", "The handle is not valid"},
	1001:  {"VIX_E_NOT_SUPPORTED_ON_HANDLE_TYPE", "The operation is not supported for this handle type"},
	1002:  {"VIX_E_TOO_MANY_HANDLES", "The handle limit has been reached"},
	2000:  {"VIX_E_NOT_FOUND", "The requested item was not found"},
	2001:  {"VIX_E_TYPE_MISMATCH", "The value has the wrong type"},
	2002:  {"VIX_E_INVALID_XML", "The XML document is invalid"},
	3000:  {"VIX_E_TIMEOUT_WAITING_FOR_TOOLS", "Timed out waiting for guest tools"},
	3001:  {"VIX_E_UNRECOGNIZED_COMMAND", "The command was not recognized"},
	3003:  {"VIX_E_OP_NOT_SUPPORTED_ON_GUEST", "The guest does not support the operation"},
	3004:  {"VIX_E_PROGRAM_NOT_STARTED", "The program could not be started"},
	3005:  {"VIX_E_CANNOT_START_READ_ONLY_VM", "A read-only virtual machine cannot be started"},
	3006:  {"VIX_E_VM_NOT_RUNNING", "The virtual machine is not running"},
	3007:  {"VIX_E_VM_IS_RUNNING", "The virtual machine is running"},
	3008:  {"VIX_E_CANNOT_CONNECT_TO_VM", "Connecting to the virtual machine failed"},
	3009:  {"VIX_E_POWEROP_SCRIPTS_NOT_AVAILABLE", "Power operation scripts are not available"},
	3010:  {"VIX_E_NO_GUEST_OS_INSTALLED", "No guest operating system is installed"},
	3011:  {"VIX_E_VM_INSUFFICIENT_HOST_MEMORY", "The host does not have enough memory"},
	3012:  {"VIX_E_SUSPEND_ERROR", "The virtual machine could not be suspended"},
	3013:  {"VIX_E_VM_NOT_ENOUGH_CPUS", "The host does not have enough processors"},
	3014:  {"VIX_E_HOST_USER_PERMISSIONS", "Insufficient permissions on the host"},
	3015:  {"VIX_E_GUEST_USER_PERMISSIONS", "Insufficient permissions in the guest"},
	3016:  {"VIX_E_TOOLS_NOT_RUNNING", "Guest tools are not running"},
	3017:  {"VIX_E_GUEST_OPERATIONS_PROHIBITED", "Guest operations are prohibited"},
	3018:  {"VIX_E_ANON_GUEST_OPERATIONS_PROHIBITED", "Anonymous guest operations are prohibited"},
	3019:  {"VIX_E_ROOT_GUEST_OPERATIONS_PROHIBITED", "Privileged guest operations are prohibited"},
	3023:  {"VIX_E_MISSING_ANON_GUEST_ACCOUNT", "The anonymous guest account is missing"},
	3024:  {"VIX_E_CANNOT_AUTHENTICATE_WITH_GUEST", "Guest authentication failed"},
	3025:  {"VIX_E_UNRECOGNIZED_COMMAND_IN_GUEST", "The guest did not recognize the command"},
	3026:  {"VIX_E_CONSOLE_GUEST_OPERATIONS_PROHIBITED", "Console guest operations are prohibited"},
	3027:  {"VIX_E_MUST_BE_CONSOLE_USER", "Only the console user may perform this operation"},
	3028:  {"VIX_E_VMX_MSG_DIALOG_AND_NO_UI", "The virtual machine is waiting for input and no UI is available"},
	3031:  {"VIX_E_OPERATION_NOT_ALLOWED_FOR_LOGIN_TYPE", "Not allowed for this login type"},
	3032:  {"VIX_E_LOGIN_TYPE_NOT_SUPPORTED", "The login type is not supported"},
	3033:  {"VIX_E_EMPTY_PASSWORD_NOT_ALLOWED_IN_GUEST", "Empty passwords are not allowed in the guest"},
	3034:  {"VIX_E_INTERACTIVE_SESSION_NOT_PRESENT", "No interactive session is present"},
	3035:  {"VIX_E_INTERACTIVE_SESSION_USER_MISMATCH", "The interactive session belongs to another user"},
	3041:  {"VIX_E_CANNOT_POWER_ON_VM", "The virtual machine could not be powered on"},
	3043:  {"VIX_E_NO_DISPLAY_SERVER", "No display server is available"},
	3046:  {"VIX_E_TOO_MANY_LOGONS", "Too many logons"},
	3047:  {"VIX_E_INVALID_AUTHENTICATION_SESSION", "The authentication session is invalid"},
	4000:  {"VIX_E_VM_NOT_FOUND", "The virtual machine was not found"},
	4001:  {"VIX_E_NOT_SUPPORTED_FOR_VM_VERSION", "Not supported for this virtual machine version"},
	4002:  {"VIX_E_CANNOT_READ_VM_CONFIG", "The virtual machine configuration could not be read"},
	4003:  {"VIX_E_TEMPLATE_VM", "The operation is not allowed on a template"},
	4004:  {"VIX_E_VM_ALREADY_LOADED", "The virtual machine is already loaded"},
	4006:  {"VIX_E_VM_ALREADY_UP_TO_DATE", "The virtual machine is already up to date"},
	4011:  {"VIX_E_VM_UNSUPPORTED_GUEST", "The guest operating system is not supported"},
	6000:  {"VIX_E_UNRECOGNIZED_PROPERTY", "The property is not recognized"},
	6001:  {"VIX_E_INVALID_PROPERTY_VALUE", "The property value is invalid"},
	6002:  {"VIX_E_READ_ONLY_PROPERTY", "The property is read-only"},
	6003:  {"VIX_E_MISSING_REQUIRED_PROPERTY", "A required property is missing"},
	6004:  {"VIX_E_INVALID_SERIALIZED_DATA", "Serialized data is invalid"},
	6005:  {"VIX_E_PROPERTY_TYPE_MISMATCH", "The property has the wrong type"},
	8000:  {"VIX_E_BAD_VM_INDEX", "The virtual machine index is out of range"},
	10000: {"VIX_E_INVALID_MESSAGE_HEADER", "A message header is invalid"},
	10001: {"VIX_E_INVALID_MESSAGE_BODY", "A message body is invalid"},
	13000: {"VIX_E_SNAPSHOT_INVAL", "The snapshot request is invalid"},
	13001: {"VIX_E_SNAPSHOT_DUMPER", "The snapshot state could not be saved"},
	13002: {"VIX_E_SNAPSHOT_DISKLIB", "A disk error occurred during a snapshot operation"},
	13003: {"VIX_E_SNAPSHOT_NOTFOUND", "The snapshot was not found"},
	13004: {"VIX_E_SNAPSHOT_EXISTS", "A snapshot with that name exists already"},
	13005: {"VIX_E_SNAPSHOT_VERSION", "The snapshot version is not supported"},
	13006: {"VIX_E_SNAPSHOT_NOPERM", "Insufficient permissions for the snapshot operation"},
	13007: {"VIX_E_SNAPSHOT_CONFIG", "The snapshot configuration is invalid"},
	13008: {"VIX_E_SNAPSHOT_NOCHANGE", "The snapshot state has not changed"},
	13009: {"VIX_E_SNAPSHOT_CHECKPOINT", "The snapshot checkpoint could not be saved"},
	13010: {"VIX_E_SNAPSHOT_LOCKED", "Snapshot operations are locked"},
	13011: {"VIX_E_SNAPSHOT_INCONSISTENT", "The snapshot tree is inconsistent"},
	13012: {"VIX_E_SNAPSHOT_NAMETOOLONG", "The snapshot name is too long"},
	13013: {"VIX_E_SNAPSHOT_VIXFILE", "The snapshot metadata file is invalid"},
	13014: {"VIX_E_SNAPSHOT_DISKLOCKED", "A disk of the snapshot is locked"},
	13015: {"VIX_E_SNAPSHOT_DUPLICATEDDISK", "A disk appears more than once in the snapshot"},
	13016: {"VIX_E_SNAPSHOT_INDEPENDENTDISK", "Independent disks cannot be snapshotted"},
	13017: {"VIX_E_SNAPSHOT_NONUNIQUE_NAME", "The snapshot name is not unique"},
	13018: {"VIX_E_SNAPSHOT_MEMORY_ON_INDEPENDENT_DISK", "Memory snapshots are not allowed with independent disks"},
	13019: {"VIX_E_SNAPSHOT_MAXSNAPSHOTS", "The maximum number of snapshots was reached"},
	13020: {"VIX_E_SNAPSHOT_MIN_FREE_SPACE", "Not enough free space for the snapshot"},
	13021: {"VIX_E_SNAPSHOT_HIERARCHY_TOODEEP", "The snapshot hierarchy is too deep"},
	13024: {"VIX_E_SNAPSHOT_NOT_REVERTABLE", "The snapshot cannot be reverted to"},
	14003: {"VIX_E_HOST_DISK_INVALID_VALUE", "The host disk value is invalid"},
	14004: {"VIX_E_HOST_DISK_SECTORSIZE", "The host disk sector size is not supported"},
	14005: {"VIX_E_HOST_FILE_ERROR_EOF", "Unexpected end of file"},
	14006: {"VIX_E_HOST_NETBLKDEV_HANDSHAKE", "The network block device handshake failed"},
	14007: {"VIX_E_HOST_SOCKET_CREATION_ERROR", "A network socket could not be created"},
	14008: {"VIX_E_HOST_SERVER_NOT_FOUND", "The server was not found"},
	14009: {"VIX_E_HOST_NETWORK_CONN_REFUSED", "The server refused the connection"},
	14010: {"VIX_E_HOST_TCP_SOCKET_ERROR", "A TCP socket error occurred"},
	14011: {"VIX_E_HOST_TCP_CONN_LOST", "The TCP connection was lost"},
	14012: {"VIX_E_HOST_NBD_HASHFILE_VOLUME", "The NBD hash file volume is invalid"},
	14013: {"VIX_E_HOST_NBD_HASHFILE_INIT", "The NBD hash file could not be initialized"},
	14014: {"VIX_E_HOST_SERVER_SHUTDOWN", "The server is shutting down"},
	14015: {"VIX_E_HOST_SERVER_NOT_AVAILABLE", "The server is not available"},
	16000: {"VIX_E_DISK_INVAL", "The disk request is invalid"},
	16001: {"VIX_E_DISK_NOINIT", "The disk library is not initialized"},
	16002: {"VIX_E_DISK_NOIO", "The disk could not be accessed"},
	16003: {"VIX_E_DISK_PARTIALCHAIN", "The disk chain is incomplete"},
	16006: {"VIX_E_DISK_NEEDSREPAIR", "The disk needs repair"},
	16007: {"VIX_E_DISK_OUTOFRANGE", "The request is outside the disk"},
	16008: {"VIX_E_DISK_CID_MISMATCH", "The disk chain identifiers do not match"},
	16009: {"VIX_E_DISK_CANTSHRINK", "The disk cannot be shrunk"},
	16010: {"VIX_E_DISK_PARTMISMATCH", "The partition table does not match the disk"},
	16011: {"VIX_E_DISK_UNSUPPORTEDDISKVERSION", "The disk format version is not supported"},
	16012: {"VIX_E_DISK_OPENPARENT", "A parent disk could not be opened"},
	16013: {"VIX_E_DISK_NOTSUPPORTED", "The disk operation is not supported"},
	16014: {"VIX_E_DISK_NEEDKEY", "The disk requires an encryption key"},
	16015: {"VIX_E_DISK_NOKEYOVERRIDE", "The disk encryption key cannot be overridden"},
	16016: {"VIX_E_DISK_NOTENCRYPTED", "The disk is not encrypted"},
	16017: {"VIX_E_DISK_NOKEY", "No encryption key was supplied"},
	16018: {"VIX_E_DISK_INVALIDPARTITIONTABLE", "The partition table is not valid"},
	16019: {"VIX_E_DISK_NOTNORMAL", "The disk is not a normal disk"},
	16020: {"VIX_E_DISK_NOTENCDESC", "The disk descriptor is not encrypted"},
	16022: {"VIX_E_DISK_NEEDVMFS", "The disk must reside on VMFS"},
	16024: {"VIX_E_DISK_RAWTOOBIG", "The raw device is too large"},
	16027: {"VIX_E_DISK_TOOMANYOPENFILES", "Too many disk files are open"},
	16028: {"VIX_E_DISK_TOOMANYREDO", "The disk chain has too many redo logs"},
	16029: {"VIX_E_DISK_RAWTOOSMALL", "The raw device is too small"},
	16030: {"VIX_E_DISK_INVALIDCHAIN", "The disk chain is invalid"},
	16052: {"VIX_E_DISK_KEY_NOTFOUND", "The metadata key was not found"},
	16053: {"VIX_E_DISK_SUBSYSTEM_INIT_FAIL", "A required subsystem failed to initialize"},
	16054: {"VIX_E_DISK_INVALID_CONNECTION", "The connection is not valid"},
	16061: {"VIX_E_DISK_ENCODING", "A character encoding error occurred"},
	16062: {"VIX_E_DISK_CANTREPAIR", "The disk cannot be repaired"},
	16063: {"VIX_E_DISK_INVALIDDISK", "The file is not a valid virtual disk"},
	16064: {"VIX_E_DISK_NOLICENSE", "No license is available for this disk operation"},
	16065: {"VIX_E_DISK_NODEVICE", "The disk device was not found"},
	16066: {"VIX_E_DISK_UNSUPPORTEDDEVICE", "The disk device is not supported"},
	16067: {"VIX_E_DISK_CAPACITY_MISMATCH", "The disk capacity does not match"},
	16068: {"VIX_E_DISK_PARENT_NOTALLOWED", "A parent disk is not allowed here"},
	16069: {"VIX_E_DISK_ATTACH_ROOTLINK", "The root link cannot be attached"},
	17000: {"VIX_E_CRYPTO_UNKNOWN_ALGORITHM", "The cryptographic algorithm is unknown"},
	17001: {"VIX_E_CRYPTO_BAD_BUFFER_SIZE", "The cryptographic buffer size is wrong"},
	17002: {"VIX_E_CRYPTO_INVALID_OPERATION", "The cryptographic operation is invalid"},
	17003: {"VIX_E_CRYPTO_RANDOM_DEVICE", "The random number source failed"},
	17004: {"VIX_E_CRYPTO_NEED_PASSWORD", "A password is required"},
	17005: {"VIX_E_CRYPTO_BAD_PASSWORD", "The password is incorrect"},
	17006: {"VIX_E_CRYPTO_NOT_IN_DICTIONARY", "The item is not in the dictionary"},
	17007: {"VIX_E_CRYPTO_NO_CRYPTO", "Cryptography is not available"},
	17008: {"VIX_E_CRYPTO_ERROR", "A cryptographic error occurred"},
	17009: {"VIX_E_CRYPTO_BAD_FORMAT", "The encrypted data is malformed"},
	17010: {"VIX_E_CRYPTO_LOCKED", "The encrypted object is locked"},
	17011: {"VIX_E_CRYPTO_EMPTY", "The encrypted object is empty"},
	17012: {"VIX_E_CRYPTO_KEYSAFE_LOCATOR", "The key locator is invalid"},
	18000: {"VIX_E_CANNOT_CONNECT_TO_HOST", "Connecting to the host failed"},
	18001: {"VIX_E_NOT_FOR_REMOTE_HOST", "Not available for remote hosts"},
	18002: {"VIX_E_INVALID_HOSTNAME_SPECIFICATION", "The host name is invalid"},
	19000: {"VIX_E_SCREEN_CAPTURE_ERROR", "Screen capture failed"},
	19001: {"VIX_E_SCREEN_CAPTURE_BAD_FORMAT", "The screen capture format is invalid"},
	19002: {"VIX_E_SCREEN_CAPTURE_COMPRESSION_FAIL", "Screen capture compression failed"},
	19003: {"VIX_E_SCREEN_CAPTURE_LARGE_DATA", "The screen capture is too large"},
	20000: {"VIX_E_GUEST_VOLUMES_NOT_FROZEN", "Guest volumes could not be frozen"},
	20001: {"VIX_E_NOT_A_FILE", "The path is not a file"},
	20002: {"VIX_E_NOT_A_DIRECTORY", "The path is not a directory"},
	20003: {"VIX_E_NO_SUCH_PROCESS", "The process does not exist"},
	20004: {"VIX_E_FILE_NAME_TOO_LONG", "The file name is too long"},
	20005: {"VIX_E_OPERATION_DISABLED", "The operation is disabled"},
	21000: {"VIX_E_TOOLS_INSTALL_NO_IMAGE", "No tools installation image was found"},
	21001: {"VIX_E_TOOLS_INSTALL_IMAGE_INACCESIBLE", "The tools installation image is not accessible"},
	21002: {"VIX_E_TOOLS_INSTALL_NO_DEVICE", "No device is available for tools installation"},
	21003: {"VIX_E_TOOLS_INSTALL_DEVICE_NOT_CONNECTED", "The tools installation device is not connected"},
	21004: {"VIX_E_TOOLS_INSTALL_CANCELLED", "Tools installation was cancelled"},
	21005: {"VIX_E_TOOLS_INSTALL_INIT_FAILED", "Tools installation could not start"},
	21006: {"VIX_E_TOOLS_INSTALL_AUTO_NOT_SUPPORTED", "Automatic tools installation is not supported"},
	21007: {"VIX_E_TOOLS_INSTALL_GUEST_NOT_READY", "The guest is not ready for tools installation"},
	21008: {"VIX_E_TOOLS_INSTALL_SIG_CHECK_FAILED", "The tools package signature check failed"},
	21009: {"VIX_E_TOOLS_INSTALL_ERROR", "Tools installation failed"},
	21010: {"VIX_E_TOOLS_INSTALL_ALREADY_UP_TO_DATE", "Tools are already up to date"},
	21011: {"VIX_E_TOOLS_INSTALL_IN_PROGRESS", "Tools installation is already in progress"},
	21012: {"VIX_E_TOOLS_INSTALL_IMAGE_COPY_FAILED", "The tools installation image could not be copied"},
	22001: {"VIX_E_WRAPPER_WORKSTATION_NOT_INSTALLED", "The hosted product is not installed"},
	22002: {"VIX_E_WRAPPER_VERSION_NOT_FOUND", "The requested version was not found"},
	22003: {"VIX_E_WRAPPER_SERVICEPROVIDER_NOT_FOUND", "The service provider was not found"},
	22004: {"VIX_E_WRAPPER_PLAYER_NOT_INSTALLED", "The player product is not installed"},
	22005: {"VIX_E_WRAPPER_RUNTIME_NOT_INSTALLED", "The runtime is not installed"},
	22006: {"VIX_E_WRAPPER_MULTIPLE_SERVICEPROVIDERS", "Multiple service providers were found"},
	24000: {"VIX_E_MNTAPI_MOUNTPT_NOT_FOUND", "The mount point was not found"},
	24001: {"VIX_E_MNTAPI_MOUNTPT_IN_USE", "The mount point is in use"},
	24002: {"VIX_E_MNTAPI_DISK_NOT_FOUND", "The disk to mount was not found"},
	24003: {"VIX_E_MNTAPI_DISK_NOT_MOUNTED", "The disk is not mounted"},
	24004: {"VIX_E_MNTAPI_DISK_IS_MOUNTED", "The disk is mounted"},
	24005: {"VIX_E_MNTAPI_DISK_NOT_SAFE", "The disk is not safe to mount"},
	24006: {"VIX_E_MNTAPI_DISK_CANT_OPEN", "The disk to mount could not be opened"},
	24007: {"VIX_E_MNTAPI_CANT_READ_PARTS", "The partitions could not be read"},
	24008: {"VIX_E_MNTAPI_UMOUNT_APP_NOT_FOUND", "The unmount helper was not found"},
	24009: {"VIX_E_MNTAPI_UMOUNT", "Unmount failed"},
	24010: {"VIX_E_MNTAPI_NO_MOUNTABLE_PARTITONS", "No mountable partitions were found"},
	24011: {"VIX_E_MNTAPI_PARTITION_RANGE", "The partition number is out of range"},
	24012: {"VIX_E_MNTAPI_PERM", "Insufficient permissions to mount"},
	24013: {"VIX_E_MNTAPI_DICT", "The mount dictionary is invalid"},
	24014: {"VIX_E_MNTAPI_DICT_LOCKED", "The mount dictionary is locked"},
	24015: {"VIX_E_MNTAPI_OPEN_HANDLES", "Handles are still open"},
	24016: {"VIX_E_MNTAPI_CANT_MAKE_VAR_DIR", "The state directory could not be created"},
	24017: {"VIX_E_MNTAPI_NO_ROOT", "Root privileges are required"},
	24018: {"VIX_E_MNTAPI_LOOP_FAILED", "The loop device operation failed"},
	24019: {"VIX_E_MNTAPI_DAEMON", "The mount daemon failed"},
	24020: {"VIX_E_MNTAPI_INTERNAL", "An internal mount error occurred"},
	24021: {"VIX_E_MNTAPI_SYSTEM", "A system error occurred while mounting"},
	24022: {"VIX_E_MNTAPI_NO_CONNECTION_DETAILS", "Connection details are missing"},
	24300: {"VIX_E_MNTAPI_INCOMPATIBLE_VERSION", "The mount library version is incompatible"},
	24301: {"VIX_E_MNTAPI_OS_ERROR", "An operating system error occurred"},
	24302: {"VIX_E_MNTAPI_DRIVE_LETTER_IN_USE", "The drive letter is in use"},
	24303: {"VIX_E_MNTAPI_DRIVE_LETTER_ALREADY_ASSIGNED", "The drive letter is already assigned"},
	24304: {"VIX_E_MNTAPI_VOLUME_NOT_MOUNTED", "The volume is not mounted"},
	24305: {"VIX_E_MNTAPI_VOLUME_ALREADY_MOUNTED", "The volume is already mounted"},
	24306: {"VIX_E_MNTAPI_FORMAT_FAILURE", "The volume could not be formatted"},
	24307: {"VIX_E_MNTAPI_NO_DRIVER", "The required driver is missing"},
	24308: {"VIX_E_MNTAPI_ALREADY_OPENED", "The volume is already open"},
	24309: {"VIX_E_MNTAPI_ITEM_NOT_FOUND", "The item was not found"},
	24310: {"VIX_E_MNTAPI_UNSUPPROTED_BOOT_LOADER", "The boot loader is not supported"},
	24311: {"VIX_E_MNTAPI_UNSUPPROTED_OS", "The operating system is not supported"},
	24312: {"VIX_E_MNTAPI_CODECONVERSION", "Character conversion failed"},
	24313: {"VIX_E_MNTAPI_REGWRITE_ERROR", "The registry could not be written"},
	24314: {"VIX_E_MNTAPI_UNSUPPORTED_FT_VOLUME", "The fault-tolerant volume is not supported"},
	24315: {"VIX_E_MNTAPI_PARTITION_NOT_FOUND", "The partition was not found"},
	24316: {"VIX_E_MNTAPI_PUTFILE_ERROR", "The file could not be written to the volume"},
	24317: {"VIX_E_MNTAPI_GETFILE_ERROR", "The file could not be read from the volume"},
	24318: {"VIX_E_MNTAPI_REG_NOT_OPENED", "The registry is not open"},
	24319: {"VIX_E_MNTAPI_REGDELKEY_ERROR", "The registry key could not be deleted"},
	24320: {"VIX_E_MNTAPI_CREATE_PARTITIONTABLE_ERROR", "The partition table could not be created"},
	24321: {"VIX_E_MNTAPI_OPEN_FAILURE", "The volume could not be opened"},
	24322: {"VIX_E_MNTAPI_VOLUME_NOT_WRITABLE", "The volume is not writable"},
	25000: {"VIX_ASYNC", "The operation will complete asynchronously"},
	26000: {"VIX_E_ASYNC_MIXEDMODE_UNSUPPORTED", "Synchronous and asynchronous I/O cannot be mixed on this handle"},
	30001: {"VIX_E_NET_HTTP_UNSUPPORTED_PROTOCOL", "The HTTP protocol is not supported"},
	30003: {"VIX_E_NET_HTTP_URL_MALFORMAT", "The URL is malformed"},
	30005: {"VIX_E_NET_HTTP_COULDNT_RESOLVE_PROXY", "The proxy host name could not be resolved"},
	30006: {"VIX_E_NET_HTTP_COULDNT_RESOLVE_HOST", "The host name could not be resolved"},
	30007: {"VIX_E_NET_HTTP_COULDNT_CONNECT", "The HTTP connection could not be established"},
	30022: {"VIX_E_NET_HTTP_HTTP_RETURNED_ERROR", "The HTTP server returned an error"},
	30028: {"VIX_E_NET_HTTP_OPERATION_TIMEDOUT", "The HTTP operation timed out"},
	30035: {"VIX_E_NET_HTTP_SSL_CONNECT_ERROR", "The TLS connection could not be established"},
	30047: {"VIX_E_NET_HTTP_TOO_MANY_REDIRECTS", "Too many HTTP redirects"},
	30200: {"VIX_E_NET_HTTP_TRANSFER", "The HTTP transfer failed"},
	30201: {"VIX_E_NET_HTTP_SSL_SECURITY", "TLS security verification failed"},
	30202: {"VIX_E_NET_HTTP_GENERIC", "An HTTP error occurred"},
}

// Name returns the symbolic name of c, or "" when the code is not defined.
func (c Code) Name() string { return catalog[c].name }

// Known reports whether c is defined by the ABI.
func (c Code) Known() bool { _, ok := catalog[c]; return ok }

// Text returns this project's description of c. Unknown codes share the
// generic VIX_E_FAIL description.
func (c Code) Text() string {
	if e, ok := catalog[c]; ok {
		return e.text
	}
	return catalog[EFail].text
}

// CodeOf extracts the code from a full 64-bit VixError value.
func CodeOf(err uint64) Code { return Code(err & 0xFFFF) }

// Codes returns every defined code; used by tests and the ledger tool.
func Codes() []Code {
	out := make([]Code, 0, len(catalog))
	for c := range catalog {
		out = append(out, c)
	}
	return out
}

// Further codes used by the implementation.
const (
	ETooManyHandles    Code = 1002
	EOperationDisabled Code = 20005
	// ENotSupportedOnRemoteObject: PrepareForAccess and EndAccess on a
	// connection to an ESXi host, as the reference answers them (D-0052).
	ENotSupportedOnRemoteObject Code = 20
)
