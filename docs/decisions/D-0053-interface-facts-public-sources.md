# D-0053 — Where the interface facts come from: public sources and the library's linking facts

- Date: 2026-11-11
- Contract: §50
- Status: accepted by the maintainer on 2026-11-11, after a review of every item of the public interface. Item 5
  (the terms) added on 2026-11-13. Item 3 amended by D-0054 on 2026-11-14.

## Context

A prospective contributor pointed out that two of the header files in VMware's VDDK package carry a "VMware
Confidential" marking, while PROVENANCE.md named VDDK's headers as the source of OpenVDDK's interface facts. They asked
whether every fact OpenVDDK uses is also available in public sources.

## The review

On 2026-11-11, 421 items of OpenVDDK's public interface were matched against public sources: 43 function prototypes, 28
types, 57 constants, 266 error codes and 27 facts about the library file.

- **400 items have a public source.**
  - Broadcom's open-source open-vm-tools has every error code with its value (`lib/include/vix.h`, LGPL-2.1) and
    every basic type name (`lib/include/vm_basic_types.h`). `vm_basic_types.h` is the marked header in question;
    Broadcom publishes it as open source. It carries three licence notices: LGPL-2.1, a BSD-style licence with two
    conditions, and CDDL-1.0. This record draws no conclusion about how they relate. OpenVDDK takes only facts from
    these files (names and values), never their text.
  - The API reference VMware ships with VDDK has 33 of the 43 prototypes and the error list.
  - VMware's *VDDK Programming Guide* (8.0 U3) has the datastore specification and the sector size.
  - nbdkit's VDDK plugin has the structure layouts, enumerations, open flags and limits.
- **About 15 of those have only weaker public sources.** For these, the only public sources are VMware's
  *NAS VAAI Development Kit 7.0.1* API reference, as posted publicly by NetApp, and projects that reproduce VMware's
  header. The items:
  - two prototypes: `GetConnectParams` and `IsAttachPossible`;
  - four constant values: `MAX_CHUNK_SIZE`, `FLAG_OPEN_COMPRESSION_MASK`, `HWVERSION_CURRENT` and `SPEC_UNKNOWN`;
  - four opaque type names;
  - four field names of the connection parameters.
- **21 items are facts of the library file VMware distributed.** They were read from its dynamic tables with
  `readelf`, `nm -D` and `objdump -T`, never from its code sections:
  - the SONAME;
  - the twelve symbol version names, their order, and which function carries which version;
  - three internal exports;
  - for the record only, the libraries it needs, its glibc floor and its build flags.

  No public document lists them. A program records the SONAME and the symbol versions when it is linked, so a
  replacement has to carry them to be loaded in its place.

No header text, code or comment was reproduced. Nothing was decompiled or disassembled.

## Decision

1. PROVENANCE.md and docs/CLEAN-ROOM.md cite the public sources above instead of VDDK's header files. A note in
   PROVENANCE.md says what they named before.
2. The linking facts stay. Binary compatibility needs them, and anyone can read them with standard tools from the
   library VMware distributed.
3. The three internal exports are counted, not named, in the public tree since D-0054; their names are recorded
   privately. A private build of the tracer keeps forwarding them, because VMware's own library needs them when the
   tracer sits in front of it. OpenVDDK itself does not export them (D-0008).
4. **The reference packages.** The maintainer downloaded them from VMware's public download page; until August 2026 it
   required no account. The terms that came with them are VMware's General Terms of 22 November 2023, shipped in the
   8.0.3 package as `doc/vmware-general-terms.pdf`. Their section 4.2 excludes publicly available information from
   the confidentiality duty.
5. **What the terms say about examining the library** (added 2026-11-13, after the same contributor asked).
   - The General Terms make the agreement the order, the General Terms, the Exhibits and the Offering-specific Notes
     (section 1.1). The General Terms themselves have no clause about examining the software.
   - The restriction is in VMware's Software Exhibit of the same date (version "Nov 2023"). The Exhibit applies when
     software is bought, and the General Terms define the software as programs licensed under an order. Its section
     2(d) forbids attempts to "reverse engineer, decompile, or otherwise attempt to derive source code", except where
     the law permits.
   - The Offering-specific Notes are VMware's Product Guide. Its November 2023 version, the one published when the
     General Terms above were dated, has no entry for VDDK or the vSphere SDKs, and neither do the December 2023 and
     April 2024 versions. Its free-software section lists other tools. Its clause on APIs, which also excludes reverse
     engineering, sits in the Workstation and Fusion sections and covers the VIX API kit those desktop products
     include. Its section 1.2 ties the applicable version to the date VMware accepts an order.
   - The reference packages were free downloads. There was no order and no purchase.
   - No step of OpenVDDK's work sought source code. The dynamic tables were read as the system's loader reads them,
     and the library's behaviour was learned by running it and observing the results. OpenVDDK reproduces behaviour,
     not code.
   - The maintainer's assessment: these terms do not restrict this work. The versions cited are the ones that came
     with the package.

## Sources (checked 2026-11-11; every link answered)

- Broadcom's open-vm-tools, pinned at commit `1634dfb42f72` (2026-05-14); `vix.h` carries the LGPL-2.1 notice,
  `vm_basic_types.h` the three notices named above. The first file is the same header
  that carries the "VMware Confidential" marking in VDDK's package; here Broadcom publishes it as open source.
  - `vm_basic_types.h`: https://github.com/vmware/open-vm-tools/blob/1634dfb42f72/open-vm-tools/lib/include/vm_basic_types.h
  - `vix.h` (every VIX error code and its value): https://github.com/vmware/open-vm-tools/blob/1634dfb42f72/open-vm-tools/lib/include/vix.h
- nbdkit's VDDK plugin (`vddk-structs.h`, `vddk-stubs.h`), pinned at commit `0614f1789913`:
  https://gitlab.com/nbdkit/nbdkit/-/tree/0614f1789913/plugins/vddk
- VMware's *VDDK Programming Guide* for 8.0.3, as archived by the Internet Archive:
  https://web.archive.org/web/20241219223812/https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-vddk-803-programming-guide.pdf
- VMware's VIX API 1.12 error-code reference, as archived:
  https://web.archive.org/web/20140126070219/https://www.vmware.com/support/developer/vix-api/vix112_reference/errors/errors.html
- VMware's *NAS VAAI Development Kit 7.0.1* API reference, as posted publicly by NetApp:
  https://opensource.netapp.com/NetApp_NFS_Plug-in_for_VMware_VAAI/2.0.1/rpmextract/nvdk-7.0.1-16850804/docs/html/distribute__vixDiskLib_8h.html
- The API reference VMware ships with VDDK: the `doc/` folder of `VMware-vix-disklib-8.0.3-23950268.x86_64.tar.gz`.
- The terms that came with the reference package: VMware General Terms, version of 22 November 2023, in the same
  package as `doc/vmware-general-terms.pdf`. Section 4.2 (Exceptions) says the confidentiality duty does not apply to
  information that "becomes publicly available through no fault of Recipient". The file's SHA-256 is
  `7ca6efae3eab3336e76dada258fafe808e0601d4ae7e5044ef1fff14be8d6758`.
- VMware's Software Exhibit to the General Terms, version "Nov 2023", as archived by the Internet Archive on
  2024-03-28 (read 2026-11-13):
  https://web.archive.org/web/20240328224057/https://www.vmware.com/content/dam/digitalmarketing/vmware/en/pdf/agreements/vmware-software-exhibit.pdf
  The archived file's SHA-256 is `947634c3db43544a85557e3adb73abf65fe8ab5ecdf4d4dbde90f10c35b128cb`.
- VMware's Product Guide (the Offering-specific Notes), version "November 2023", as archived on 2023-11-10 (read
  2026-11-13; the December 2023 and April 2024 versions, archived 2024-01-27 and 2024-03-31, were checked too):
  https://web.archive.org/web/20231110180025/https://www.vmware.com/content/dam/digitalmarketing/vmware/en/pdf/product/vmware-product-guide.pdf
  The archived file's SHA-256 is `c15f91776220c853e9f6de1679e26f5d6bf1b08a3dd17a336e428c5c26a49fbe`.
- The maintainer keeps a copy of each of these documents, so they can be checked against the values above if a link
  stops working.
