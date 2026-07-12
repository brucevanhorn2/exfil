# Changelog

All notable changes to this project are documented here. Format loosely follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

## [0.1.0] - 2026-07-11

First functional MVP, verified end-to-end against a live SSH host.

### Added
- Dual-pane file browser, local and remote (SFTP) over the same `FileSystem` interface
- SSH/SFTP connection via a Site Manager (`s`), with saved hosts in `~/.config/exfil/hosts.yaml`
- Add/edit hosts from within the app (`n`/`e`), edits keyed by host name to avoid stale-index corruption
- Directional transfers: `→` pushes local→remote, `←` pulls remote→local, regardless of pane focus
- Concurrent transfer worker pool (3 workers) with live progress bars and speed calculation
- Transfer queue pane with a capped height so it can't push the layout off-screen
- About screen (`?`) with logo, build-time version (via `git describe`), and license
- GitHub Actions CI (build, `go vet`, `gofmt` check)
- MIT license

### Fixed
- `Tab` focus toggle converging both panes to the same focus state instead of alternating
- Selection checkmarks hidden by the cursor arrow on the same row
- Browser panes collapsing instead of filling their assigned width/height
- `Back()` producing a doubled leading slash in the displayed path
- Remote pane never getting an initial directory listing before connecting, breaking local-to-local testing

[Unreleased]: https://github.com/brucevanhorn2/exfil/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/brucevanhorn2/exfil/releases/tag/v0.1.0
