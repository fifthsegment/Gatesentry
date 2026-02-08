# GateSentry Docker Deployment Guide

## Overview

GateSentry is a DNS-based parental controls and web filtering system for home networks.
It replaces Pi-Hole with better parental controls, automatic device discovery, and a
simpler UI. This guide covers deploying GateSentry as a Docker container on your home
network.

### What GateSentry does

- **DNS server** (port 53) — filters ads, malware, and inappropriate content
- **Device discovery** — automatically identifies every device on your network
- **Web admin UI** (port 80) — manage settings, view devices, control access
- **HTTP(S) proxy** (port 10413) — optional content-level filtering with MITM inspection
- **RFC 2136 DDNS** — receives dynamic DNS updates from your DHCP server

### Architecture

```
    Internet
        │
   ┌────┴────┐
   │  Router  │ ← DHCP server (assigns IPs, sets DNS to GateSentry)
   │          │ ← Sends RFC 2136 DDNS updates to GateSentry (if capable)
   └────┬────┘
        │
   Home Network
        │
   ┌────┴──────────────┐
   │  GateSentry Host  │ ← Docker host (Raspberry Pi, NUC, old laptop, VM)
   │  (Docker)         │
   │                   │
   │  :53   DNS        │ ← Every device on the network queries this
   │  :80   Web UI     │ ← Admin dashboard (http://gatesentry.local)
   │  :10413 Proxy     │ ← Optional HTTPS filtering proxy
   └───────────────────┘
```

---

## Prerequisites

- Docker Engine 20.10+ and Docker Compose v2
- A Linux host (Raspberry Pi 4+, Intel NUC, VM, any x86_64 or ARM64 machine)
- The host must NOT already have a DNS server on port 53 (check: `ss -tlnp | grep :53`)
- If `systemd-resolved` occupies port 53, see [Freeing Port 53](#freeing-port-53-systemd-resolved)

---

## Quick Start

### 1. Clone and build

```bash
git clone <your-repo-url> gatesentry
cd gatesentry

# Install UI dependencies (first time only)
cd ui && npm install && cd ..

# Build everything (Svelte UI → embed into Go → static binary)
./build.sh

# Start the container
docker compose up -d --build
```

`build.sh` builds the Svelte UI, copies the dist into Go's embed directory, then compiles
a static Go binary with everything baked in. The Docker image is just Alpine + that binary
(~30MB).

### 2. Configure your router's DHCP

Set GateSentry's IP as the **DNS server** for your network:

| Router Type | Setting Location |
|-------------|-----------------|
| Most routers | DHCP settings → DNS server → set to GateSentry host IP |
| pfSense | Services → DHCP Server → DNS servers |
| Ubiquiti | Settings → Networks → DHCP Name Server |
| ISP router | Usually under LAN/DHCP settings |

After changing the DNS server, devices will start using GateSentry as they renew their
DHCP leases (or immediately after reconnecting to Wi-Fi).

### 3. Verify it works

```bash
# From any device on the network, query GateSentry directly
dig @<gatesentry-ip> google.com

# Check the admin UI
open http://<gatesentry-ip>
```

---

## docker-compose.yml Reference

```yaml
services:
  gatesentry:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: gatesentry
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./docker_root:/usr/local/gatesentry/gatesentry
    environment:
      - TZ=Asia/Singapore
```

The Dockerfile is intentionally minimal — it copies the pre-built binary into an Alpine
image. All compilation (Node + Go) happens on the host via `build.sh`. This keeps the
Docker image tiny (~30MB) and the build fast.

### Why `network_mode: host`?

GateSentry **must** use host networking. This is not optional. Here's why:

| Feature | Requires host networking? | Why |
|---------|--------------------------|-----|
| DNS server on :53 | Recommended | Avoids port mapping complexity |
| See real client IPs | **Yes** | Bridge mode shows all clients as 172.17.0.1 |
| Passive device discovery | **Yes** | Needs real source IPs from DNS queries |
| mDNS/Bonjour discovery | **Yes** | Multicast (224.0.0.251) doesn't cross Docker NAT |
| DDNS from router | Recommended | Router can target GateSentry directly |
| Per-device filtering | **Yes** | Must identify which device is querying |

Pi-Hole uses the same approach for the same reasons.

### Volume mount

```yaml
volumes:
  - ./docker_root:/usr/local/gatesentry/gatesentry
```

The `docker_root/` directory on the host stores all persistent data:
- `settings.db` — BuntDB database (settings, rules, custom DNS entries)
- Device inventory database
- MITM CA certificate and key (if HTTPS filtering is enabled)
- Logs

**Back up this directory** to preserve your configuration.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TZ` | `UTC` | Timezone for time-based access rules (e.g., `America/New_York`) |
| `GS_DEBUG_LOGGING` | `false` | Enable verbose proxy debug logging |
| `GS_MAX_SCAN_SIZE_MB` | `10` | Max content size to scan (MB). Reduce on low-memory devices |
| `GS_TRANSPARENT_PROXY` | `true` | Set to `false` to disable transparent proxy |
| `GS_TRANSPARENT_PROXY_PORT` | auto | Custom port for transparent proxy listener |
| `GS_ADMIN_PORT` | `80` | Port for the web admin UI |
| `GS_BASE_PATH` | `/gatesentry` | URL base path prefix (set to `/` for root-level access) |

---

## Reverse Proxy Deployment

If GateSentry runs behind a reverse proxy (e.g., on a NAS), set `GS_ADMIN_PORT` so
the UI listens on a non-privileged port. The default `GS_BASE_PATH=/gatesentry` already
works — just point your reverse proxy at it.

### Example: Nginx reverse proxy at `https://www.example.com/gatesentry`

**docker-compose.yml:**
```yaml
environment:
  - GS_ADMIN_PORT=8080
  # GS_BASE_PATH defaults to /gatesentry — no need to set it
```

**Nginx config:**
```nginx
location /gatesentry/ {
    proxy_pass http://127.0.0.1:8080/gatesentry/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

The base path prefixes **all** routes (UI pages, API, and static assets) so everything
goes through the same origin — no CORS configuration needed.

| URL | What it serves |
|-----|---------------|
| `/gatesentry/` | Admin UI (Svelte SPA) |
| `/gatesentry/api/...` | REST API |
| `/gatesentry/fs/...` | Static assets (JS, CSS) |
| `/gatesentry/login` | SPA login route |
| `/` | 302 redirect → `/gatesentry/` |

### Standalone at root path

To serve GateSentry at the root (no path prefix), set `GS_BASE_PATH=/`:
```yaml
environment:
  - GS_BASE_PATH=/
```
Then the UI is at `http://gatesentry.local/`, API at `http://gatesentry.local/api/...`, etc.

---

## DHCP Server Integration (RFC 2136 DDNS)

### How it works

When your DHCP server assigns an IP address to a device, it can notify GateSentry via
RFC 2136 Dynamic DNS UPDATE messages. This is the standard protocol for DHCP-DNS
integration — the same mechanism used by enterprise networks worldwide.

```
Device connects to Wi-Fi
    → Router's DHCP assigns 192.168.1.42 to "Viviennes-iPad"
    → Router sends DNS UPDATE to GateSentry:
        "viviennes-ipad.local  A  192.168.1.42"
    → GateSentry updates its device inventory
    → "viviennes-ipad.local" now resolves on the network
```

### TSIG Authentication

DDNS updates **must** be authenticated with TSIG (Transaction Signature) to prevent
any device on the network from injecting DNS records.

#### Generate a TSIG key

```bash
# Generate a random HMAC-SHA256 key
tsig-keygen -a hmac-sha256 dhcp-key
```

This outputs:
```
key "dhcp-key" {
    algorithm hmac-sha256;
    secret "YWJjZGVmMTIzNDU2Nzg5MGFiY2RlZjEyMzQ1Njc4OTA=";
};
```

Use the same key on both the DHCP server and GateSentry.

#### Configure GateSentry

In the GateSentry admin UI (Settings → DNS → DDNS):
- **Enable DDNS**: On
- **Zone**: `local` (or your preferred local zone)
- **TSIG Key Name**: `dhcp-key`
- **TSIG Algorithm**: `hmac-sha256`
- **TSIG Secret**: `YWJjZGVmMTIzNDU2Nzg5MGFiY2RlZjEyMzQ1Njc4OTA=`

### Router-Specific DDNS Configuration

#### pfSense

1. Go to **Services → DNS Resolver → General Settings**
2. Enable DHCP Registration
3. Go to **Services → DHCP Server → [interface]**
4. Under "DNS Server", enter GateSentry's IP
5. Enable "DDNS" and configure:
   - Key name: `dhcp-key`
   - Key algorithm: HMAC-SHA256
   - Key: `YWJjZGVmMTIzNDU2Nzg5MGFiY2RlZjEyMzQ1Njc4OTA=`
   - Server: GateSentry's IP

#### ISC DHCP (dhcpd)

Add to `/etc/dhcp/dhcpd.conf`:

```
key "dhcp-key" {
    algorithm hmac-sha256;
    secret "YWJjZGVmMTIzNDU2Nzg5MGFiY2RlZjEyMzQ1Njc4OTA=";
};

zone local. {
    primary <gatesentry-ip>;
    key dhcp-key;
}

# Enable DDNS updates
ddns-updates on;
ddns-update-style interim;
ddns-domainname "local.";
ddns-rev-domainname "in-addr.arpa.";
```

#### Kea DHCP

Add to your Kea configuration:

```json
{
    "Dhcp4": {
        "dhcp-ddns": {
            "enable-updates": true,
            "server-ip": "<gatesentry-ip>",
            "server-port": 53,
            "qualifying-suffix": "local.",
            "override-client-update": true
        }
    }
}
```

With TSIG in the D2 (DHCP-DDNS) configuration:

```json
{
    "DhcpDdns": {
        "tsig-keys": [
            {
                "name": "dhcp-key",
                "algorithm": "HMAC-SHA256",
                "secret": "YWJjZGVmMTIzNDU2Nzg5MGFiY2RlZjEyMzQ1Njc4OTA="
            }
        ],
        "forward-ddns": {
            "ddns-domains": [
                {
                    "name": "local.",
                    "dns-servers": [
                        { "ip-address": "<gatesentry-ip>", "port": 53 }
                    ],
                    "key-name": "dhcp-key"
                }
            ]
        }
    }
}
```

#### dnsmasq

dnsmasq does not support RFC 2136 DDNS natively. However, GateSentry's passive discovery
(Tier 3) automatically detects devices from their DNS queries — no DDNS required.

#### Consumer routers (ASUS, Netgear, TP-Link, ISP boxes)

Most consumer routers don't support RFC 2136 DDNS. This is fine — GateSentry still
discovers devices through:
- **Passive discovery** — every DNS query reveals the client's IP address
- **mDNS/Bonjour** — Apple devices, printers, Chromecasts announce themselves

DDNS is a bonus for power users with capable routers, not a requirement.

---

## mDNS / Bonjour Discovery

GateSentry listens for mDNS multicast announcements to automatically discover devices
that advertise services via Bonjour/Zeroconf:

- Apple devices (iPhones, iPads, Macs, Apple TVs)
- Printers (AirPrint)
- Chromecasts and smart speakers
- IoT devices (HomeKit, etc.)

### Requirements

- `network_mode: host` in docker-compose.yml (already set)
- No other mDNS responder on port 5353 (Avahi, etc.)

If Avahi is running on the host:
```bash
sudo systemctl stop avahi-daemon
sudo systemctl disable avahi-daemon
```

### What if I can't use host networking?

If you must use bridge networking (rare), mDNS discovery and passive device identification
won't work. DDNS from a capable router still works (target the host's IP with port mapping).
The DNS server functions normally — you just lose device discovery features.

---

## Common Setup Tasks

### Freeing port 53 (systemd-resolved)

On Ubuntu/Debian, `systemd-resolved` listens on port 53. Free it:

```bash
# Check if systemd-resolved is using port 53
sudo ss -tlnp | grep :53

# Option 1: Disable the stub listener (recommended)
sudo sed -i 's/#DNSStubListener=yes/DNSStubListener=no/' /etc/systemd/resolved.conf
sudo systemctl restart systemd-resolved

# Option 2: Disable systemd-resolved entirely
sudo systemctl stop systemd-resolved
sudo systemctl disable systemd-resolved

# Set a manual DNS server in /etc/resolv.conf
echo "nameserver 8.8.8.8" | sudo tee /etc/resolv.conf
```

### Running on a Raspberry Pi

GateSentry supports ARM64 (Raspberry Pi 4, 5). The Dockerfile builds for the host
architecture automatically via Docker's multi-platform support.

```bash
# On the Pi
git clone <repo-url> gatesentry
cd gatesentry
docker compose up -d --build
```

The first build on a Pi 4 takes ~5 minutes. Subsequent builds are cached.

### Viewing logs

```bash
# Follow container logs
docker compose logs -f gatesentry

# Check device discovery activity
docker compose logs gatesentry | grep -i "device\|discovery\|ddns\|mdns"
```

### Rebuilding after code changes

```bash
# Rebuild binary and restart container
./build.sh
docker compose up -d --build
```

### Stopping GateSentry

```bash
docker compose down
```

Your data is preserved in `docker_root/`. Starting again restores all settings.

---

## Ports Reference

| Port | Protocol | Service | Required? |
|------|----------|---------|-----------|
| 53 | UDP + TCP | DNS server | **Yes** — this is the core service |
| 80 | TCP | Web admin UI | **Yes** — admin interface at http://gatesentry.local |
| 10413 | TCP | HTTP(S) filtering proxy | Optional — for content-level filtering |
| 5353 | UDP | mDNS listener (receive only) | Optional — for Bonjour device discovery |

With `network_mode: host`, all ports bind directly to the host. Ensure no other
services occupy these ports.

---

## Troubleshooting

### GateSentry won't start — port 53 in use

```bash
sudo ss -tlnp | grep :53
# If systemd-resolved: see "Freeing port 53" above
# If another DNS server: stop it first
```

### Devices not showing up in discovery

1. **Check DNS is working**: `dig @<gatesentry-ip> google.com` from a client
2. **Check router DHCP**: Ensure GateSentry's IP is set as the DNS server
3. **Wait for lease renewal**: Devices use GateSentry after their DHCP lease renews
4. **Force renew**: Disconnect/reconnect Wi-Fi on a device, then check the admin UI

### DDNS updates not arriving

1. **Check TSIG keys match**: Same key name, algorithm, and secret on both sides
2. **Check connectivity**: `dig @<gatesentry-ip> +tcp SOA local.` from the DHCP server
3. **Check logs**: `docker compose logs gatesentry | grep -i ddns`
4. **Test manually**:
   ```bash
   nsupdate -y hmac-sha256:dhcp-key:YWJj... <<EOF
   server <gatesentry-ip>
   zone local.
   update add test.local. 300 A 192.168.1.99
   send
   EOF
   ```

### mDNS not discovering devices

1. **Verify host networking**: `docker inspect gatesentry | grep NetworkMode` → should be `host`
2. **Check for port conflicts**: `ss -ulnp | grep 5353`
3. **Stop Avahi if running**: `sudo systemctl stop avahi-daemon`
4. **Note**: mDNS is supplementary. Passive discovery + DDNS are the primary methods.
