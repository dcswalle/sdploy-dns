# go-dns

[![Build & Release](https://github.com/dcswalle/sdploy-dns/actions/workflows/release.yml/badge.svg)](https://github.com/dcswalle/sdploy-dns/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/badge/go-1.24-blue.svg)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A lightweight, self-hosted DNS server written in Go with ad blocking, custom overwrites, per-client rules, HA clustering, config hot-reload, and support for DNS-over-TLS and DNS-over-HTTPS upstream resolvers.

## Features

- **Custom DNS Overwrites** — override specific domains with custom IP addresses
- **Per-Client Overwrites** — return different IPs based on the client's IP or subnet
- **Ad & Domain Blocking** — load adblock-style host files from local paths or URLs
- **Per-Client Block Lists** — apply block lists only to specific IPs or subnets
- **DNS Response Caching** — reduce upstream queries with configurable TTL and optional size limit
- **Request Coalescing** — deduplicate concurrent identical upstream queries
- **Multiple Upstream Protocols** — forward via UDP, TCP, DNS-over-TLS (DoT), or DNS-over-HTTPS (DoH)
- **Round-Robin Nameservers** — distribute queries across multiple upstream servers
- **UDP & TCP Listeners** — serves DNS on both protocols on the same address
- **Auto-Reloading Block Lists** — URL-based lists are refreshed on a configurable interval
- **Config Hot-Reload** — apply config changes at runtime without restarting (except `listen_addr`)
- **HA Clustering** — master/slave roles for centralized config distribution across nodes
- **In-Memory Block Lists** — all block lists loaded into RAM at startup for fast lookups

## Installation

### Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/dcswalle/sdploy-dns/releases) page.

**Linux** (all architectures): `go-dns-linux-<arch>` — `386`, `amd64`, `arm`, `arm64`, `loong64`, `mips`, `mipsle`, `mips64`, `mips64le`, `ppc64`, `ppc64le`, `riscv64`, `s390x`

**Windows** (all architectures): `go-dns-windows-<arch>.exe` — `386`, `amd64`, `arm64`

| Platform | Example architecture | File |
|---|---|---|
| Linux | x86_64 | `go-dns-linux-amd64` |
| Linux | ARM64 | `go-dns-linux-arm64` |
| Windows | x86_64 | `go-dns-windows-amd64.exe` |
| Windows | ARM64 | `go-dns-windows-arm64.exe` |

```bash
# Example: Linux x86_64
curl -L https://github.com/dcswalle/sdploy-dns/releases/latest/download/go-dns-linux-amd64 -o go-dns
chmod +x go-dns
sudo ./go-dns config.yml
```

### Build from Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/dcswalle/sdploy-dns.git
cd sdploy-dns
go build -o go-dns .
sudo ./go-dns config.yml
```

> **Note**: Binding to port 53 requires elevated privileges on most systems (`sudo` on Linux, Administrator on Windows).

### Environment Variables

Copy `.env.example` to `.env` and set values locally. The `.env` file is gitignored — never commit secrets such as cluster API keys.

```bash
cp .env.example .env
# edit .env with your values
```

`GO_DNS_*` variables override scalar settings in `config.yml`. Complex settings (`overwrites`, `block_lists`) remain in `config.yml`.

## Quick Start

1. Create a `config.yml`:

```yaml
listen_addr: ":53"
nameservers:
  - "8.8.8.8"
  - "8.8.4.4"
cache_ttl: 60
```

2. Run the server:

```bash
sudo ./go-dns config.yml
```

3. Test it:

```bash
dig @127.0.0.1 google.com
```

## Configuration

### Full Example

```yaml
listen_addr: ":53"              # Address and port to listen on
debug: false                    # Enable verbose logging (default: false)
log_blocks: false               # Log blocked requests (default: false)
log_overwrites: false           # Log overwritten requests (default: false)
cache_ttl: 60                   # Positive cache TTL in seconds (0 = disabled)
negative_cache_ttl: 300         # NXDOMAIN cache TTL in seconds (0 = disabled)
max_cache_size: 0               # Max cache entries (0 = unlimited)
reload_interval: 60             # Block list reload interval in minutes (0 = disabled)
fallback_dns: "8.8.8.8"         # Fallback DNS for downloading block lists
dns_check_domain: "dns.google"  # Domain used to verify DNS is working
gogc: 100                       # Go GC target percentage (0 = Go default)

nameservers:
  - "8.8.8.8"
  - "8.8.4.4"

overwrites:
  example.local: "127.0.0.1"

block_lists:
  - "hosts.txt"
```

### Nameserver Protocols

```yaml
nameservers:
  # UDP (default, port 53)
  - "8.8.8.8"

  # DNS-over-TLS
  - address: "1.1.1.1"
    protocol: "dot"       # port defaults to 853

  # DNS-over-HTTPS
  - address: "cloudflare-dns.com"
    protocol: "doh"       # port defaults to 443

  # Full DoH URL
  - address: "https://dns.google/dns-query"
    protocol: "doh"

  # TCP
  - address: "9.9.9.9"
    protocol: "tcp"
```

### Per-Client DNS Overwrites

Return different IPs depending on the client's address or subnet:

```yaml
overwrites:
  # All clients
  example.local: "127.0.0.1"

  # Subnet-based (first IP is the returned address)
  internal.local:
    ips:
      - "192.168.1.10"
    subnets:
      - "192.168.1.0/24"
      - "10.0.0.0/8"

  # Specific client IPs
  dev.local:
    ips:
      - "127.0.0.1"       # returned IP
      - "192.168.1.50"    # client IPs that receive this override
      - "192.168.1.51"
```

### Block Lists

Load adblock-style host files from local paths or URLs, with optional per-client restrictions:

```yaml
block_lists:
  # Block for all clients (file or URL)
  - "hosts.txt"
  - "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"

  # Block only for specific IPs or subnets
  - file: "hosts-malware.txt"
    ips:
      - "192.168.1.1"
    subnets:
      - "10.0.0.0/8"

  # URL-based list with subnet restriction
  - file: "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"
    subnets:
      - "192.168.1.0/24"
```

Supported block list formats:

```
# Hosts file
127.0.0.1 ads.example.com
0.0.0.0 tracking.example.com

# Domain only
malware-site.com

# Adblock format
||adserver.com^
||tracker.com$
```

Popular sources: [StevenBlack/hosts](https://github.com/StevenBlack/hosts), [AdAway](https://adaway.org/hosts.txt)

### Caching

```yaml
cache_ttl: 60            # Positive cache TTL in seconds (default: 60, 0 = disabled)
negative_cache_ttl: 300  # NXDOMAIN cache TTL in seconds (default: 300, 0 = disabled)
max_cache_size: 10000    # Maximum cache entries (default: 0 = unlimited)
```

The cache respects the minimum TTL from DNS response records and is cleaned up automatically every 30 seconds. Cache keys include domain name, query type (A, AAAA, etc.), and query class. When `max_cache_size` is reached, the oldest entries are evicted.

### Config Hot-Reload

The server watches the config file for changes and reloads automatically (300 ms debounce). On reload, nameservers, overwrites, block lists, cache settings, and logging options are applied without a restart.

**Exception:** changing `listen_addr` requires a full restart — the running process keeps the original bind address.

Cluster-specific fields (`role`, `master`, `slave`) are preserved during reload and are not overwritten by synced config on slaves.

### HA Cluster (Master / Slave)

Run multiple DNS nodes with a single source of truth for shared settings (nameservers, overwrites, block lists, etc.). The master serves DNS and exposes an HTTP API; slaves pull config from the master on a schedule.

**Master** — serves DNS and distributes config:

```yaml
role: "master"
master:
  api_addr: ":8053"       # HTTP API listen address (default: ":8053")
  api_key: ""             # Set GO_DNS_MASTER_API_KEY in .env

listen_addr: ":53"
nameservers:
  - "8.8.8.8"
block_lists:
  - "hosts.txt"
```

**Slave** — serves DNS and syncs config from the master:

```yaml
role: "slave"
slave:
  master_url: "http://10.0.0.1:8053"  # Master's HTTP API URL
  api_key: ""                          # Set GO_DNS_SLAVE_API_KEY in .env
  sync_interval: 30                    # Seconds between config pulls (default: 30)

listen_addr: ":53"
```

Slaves preserve their local `role`, `master`, `slave`, and `listen_addr` when merging config from the master.

**Master API endpoints:**

| Endpoint | Method | Description |
|---|---|---|
| `/config` | GET | Returns the master's YAML config (`ETag` / `If-None-Match` supported) |
| `/health` | GET | Returns `{"status":"ok","role":"master"}` |

Authenticate with `Authorization: Bearer <api_key>` when `api_key` is set.

### Logging

```yaml
debug: false          # All debug output
log_blocks: false     # Only blocked requests → "Blocked: ads.example.com (from 192.168.1.1)"
log_overwrites: false # Only overwritten requests → "Overwrite: example.local -> 127.0.0.1"
```

`log_blocks` and `log_overwrites` work independently of `debug`.

## Systemd Service (Linux)

Install as a systemd service for automatic startup:

```bash
sudo ./install-service.sh
```

The script:
- Copies the binary to `/usr/local/bin/go-dns-server`
- Creates config at `/etc/go-dns/config.yml`
- Installs and enables the systemd service

```bash
sudo systemctl start go-dns
sudo systemctl stop go-dns
sudo systemctl restart go-dns
sudo systemctl status go-dns

# Logs
sudo journalctl -u go-dns -f
```

To uninstall:

```bash
sudo ./uninstall-service.sh
```

## Testing

```bash
# Normal query
dig @127.0.0.1 google.com

# Blocked domain (should return NXDOMAIN or 0.0.0.0)
dig @127.0.0.1 ads.example.com

# Custom overwrite
dig @127.0.0.1 example.local
```

## Contributing

Pull requests are welcome. For significant changes, please open an issue first to discuss what you'd like to change.

## Releasing

For maintainers: see [RELEASING.md](RELEASING.md) for instructions on creating a new release.

## License

[MIT](LICENSE)
