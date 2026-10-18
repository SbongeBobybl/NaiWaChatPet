# Everything SDK wrapper

DeskBox bundles only the official Everything SDK IPC wrapper. It does not
bundle, install, configure, or start the Everything application itself.

- Source: https://www.voidtools.com/Everything-SDK.zip
- Retrieved: 2026-08-25
- License: MIT, reproduced in `LICENSE.txt`
- `Everything64.dll` SHA-256:
  `81B5BE18126ACD2C2B913F8F4A821E476B18393CDD3DEBD03387C50AFD8DB88F`
- `EverythingARM64.dll` SHA-256:
  `8531EA393677DD8FD37BED7420AC93344CD458B9A1324BA65C4A75D024D61886`

MSBuild copies exactly one architecture-matching file to the application
payload as `EverythingSdk.dll`, which is the fixed P/Invoke name used by
`EverythingNativeMethods`.
