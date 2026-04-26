# nft-blocker

A lightweight Go service that dynamically blocks internet access for groups of devices using nftables named sets. It provides a sleek web UI for managing blocks with timed or permanent durations.

## Features

- **Group-based blocking**: Define groups of MAC addresses (e.g. "Kids Devices", "Guest Network") and block/unblock them with one click
- **Timed blocks**: Block a group for 15 minutes, 1 hour, 2 hours, 12 hours, or forever
- **Block all traffic**: Emergency button to block all forwarded traffic on a network interface
- **Persistent state**: Block state survives service restarts via a YAML state file
- **Dynamic nftables sets**: Uses named sets — no rules are added/removed at runtime, only set elements
- **Single binary**: Embedded web UI, no external files needed
- **Own nftables table**: Creates `inet nft_blocker` — does not interfere with your existing firewall rules

## Prerequisites

- Linux with nftables (`nft` command available)
- Go 1.21+ (for building)
- Root privileges (required for nftables manipulation)

## Building

```bash
go build -o nft-blocker .
```

## Configuration

Create a `config.yaml`:

```yaml
password: "your-secret-password"
listen: ":8081"
interface: "br_lan"           # network interface for "block all" feature
state_file: "state.yaml"     # where to persist block state

groups:
  kids:
    display_name: "Kids Devices"
    mac_addresses:
      - "2C:98:11:1F:46:3D"
      - "AA:BB:CC:DD:EE:FF"
  guests:
    display_name: "Guest Network"
    mac_addresses:
      - "11:22:33:44:55:66"
```

### Config Fields

| Field | Default | Description |
|---|---|---|
| `password` | (required) | Shared password for web UI authentication |
| `listen` | `:8081` | HTTP listen address |
| `interface` | `br_lan` | Network interface for the "block all" feature |
| `state_file` | `state.yaml` | Path to the persisted state file |
| `groups` | (required) | Map of group name → display name + MAC addresses |

## Running

```bash
sudo ./nft-blocker -config config.yaml
```

The service must run as root to manage nftables rules. Open `http://<your-server>:8081` in a browser and enter the configured password.

## How It Works

On startup, the service creates a dedicated nftables table:

```
table inet nft_blocker {
    set blocked_ifaces { type ifname; }
    set group_kids     { type ether_addr; }
    set group_guests   { type ether_addr; }

    chain forward {
        type filter hook forward priority 0; policy accept;
        iifname @blocked_ifaces counter drop
        ether saddr @group_kids counter drop
        ether saddr @group_guests counter drop
    }
}
```

- **Blocking a group** adds MAC addresses to the corresponding named set
- **Unblocking** flushes the set
- **Block all** adds the interface name to `blocked_ifaces`
- Rules are static; only set membership changes at runtime

You can inspect the state at any time:

```bash
nft list table inet nft_blocker
nft list set inet nft_blocker group_kids
```

## Deployment with systemd

Create `/etc/systemd/system/nft-blocker.service`:

```ini
[Unit]
Description=NFT Blocker - Internet Access Control
After=network.target nftables.service

[Service]
Type=simple
ExecStart=/opt/nft-blocker/nft-blocker -config /opt/nft-blocker/config.yaml
WorkingDirectory=/opt/nft-blocker
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nft-blocker
```

## API Reference

All API endpoints require authentication (session cookie from `/login`).

| Method | Path | Body | Description |
|---|---|---|---|
| `POST` | `/login` | `{"password": "..."}` | Authenticate, returns session cookie |
| `GET` | `/api/status` | — | Get all group states and block-all status |
| `POST` | `/api/block` | `{"group": "kids", "duration": "1h"}` | Block a group. Duration: `15m`, `1h`, `2h`, `12h`, or `forever` |
| `POST` | `/api/unblock` | `{"group": "kids"}` | Immediately unblock a group |
| `POST` | `/api/block-all` | — | Block all traffic on the configured interface |
| `POST` | `/api/unblock-all` | — | Unblock all traffic |

## License

MIT
