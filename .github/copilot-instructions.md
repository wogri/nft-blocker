# Project Guidelines

## Architecture

Single-binary Go service that manages nftables named sets to dynamically block/unblock groups of devices by MAC address. The service owns its own `inet nft_blocker` table — it must never modify other nftables tables.

Key components:
- `config.go` — YAML config loading (`Config`, `Group` structs)
- `state.go` — Mutex-protected state with atomic YAML persistence (`State`, `GroupState`, `BlockAllState`)
- `nftables.go` — nftables management via `nft` CLI (not netlink); creates table/sets/chain, adds/flushes set elements
- `handlers.go` — HTTP handlers, session auth, `TimerManager` for auto-unblock
- `main.go` — Entry point, boot recovery (restores blocks + timers from state file)
- `web/index.html` — Embedded UI (dark theme, vanilla JS, no frameworks)

## Build and Test

```sh
go build -o nft-blocker .                    # native build
make linux                                    # static cross-compile for x86_64
```

The binary must run as root on a Linux box with `nft` available. nftables commands cannot be tested on macOS — only compilation can be verified locally.

## Conventions

- All nftables interactions go through `runNft()` or `runNftStdin()` helpers — never call `exec.Command("nft", ...)` directly elsewhere
- State mutations must be protected by `State.mu` and followed by `State.Save()`
- The `__block_all__` key is reserved in `TimerManager` for the block-all timer
- The UI is a single embedded HTML file using `go:embed` — no npm, no build step
- Config uses `gopkg.in/yaml.v3`; no other external dependencies
