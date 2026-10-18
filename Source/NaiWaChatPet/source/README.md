# CodexPet reconstructed Go project

This project is a maintainable reconstruction based on static behavior evidence from CodexPet V10, its extracted PNG resources, and the supplied original video. The user confirmed that V10 was produced by editing resources embedded in the V8 executable; no standalone Go project exactly matching V10 was saved. This source is a reconstruction, not recovered original source code.

The Windows host, animation state machine, layered compositor, safe file-transfer path, runtime configuration loader, and tests live in this directory. The release package contains the original V10 EXE and video, extracted resources, repaired frames, build instructions, previews, and audit reports.

Run `go test ./...` and `go vet ./...` to check the source. To check the embedded asset bundle, run `go run ./cmd/codexpet -check`. The packaged `build/build.ps1` rebuilds the standalone Windows GUI executable from `assets/fixed`.

Automated checks and a Windows EXE build do not verify interactive GUI behavior. Mouse input, file drops, peer interaction, topmost behavior, and multi-monitor DPI still require desktop acceptance testing.
