# GateSentry

**DNS-based parental controls, ad blocking, and web filtering for your home network.**

[![Docker Pulls](https://img.shields.io/docker/pulls/jbarwick/gatesentry)](https://hub.docker.com/r/jbarwick/gatesentry)
[![GitHub](https://img.shields.io/badge/source-github.com%2Fjbarwick%2FGatesentry-blue?logo=github)](https://github.com/jbarwick/Gatesentry)

**Source:** [github.com/jbarwick/Gatesentry](https://github.com/jbarwick/Gatesentry)

> Built from a fork of [fifthsegment/Gatesentry](https://github.com/fifthsegment/Gatesentry)
> with enhanced containerization, automatic device discovery, configurable root path for
> reverse proxy deployments, and RFC 2136 DDNS support.

---

## What is GateSentry?

GateSentry is an open-source DNS server + HTTP(S) filtering proxy with a built-in
web admin UI. Point your router's DHCP at it and every device on your network gets
ad blocking, malware protection, and parental controls — no per-device configuration.

### Key Features

- 🛡️ **DNS filtering** — block ads, malware, and inappropriate content at the network level
- 🔍 **HTTPS inspection** — optional SSL/MITM proxy for content-level filtering
- 📱 **Automatic device discovery** — identifies every device via passive DNS, mDNS/Bonjour, and RFC 2136 DDNS
- 🎛️ **Web admin UI** — manage rules, view devices, control access from any browser
- 🏠 **Per-device controls** — set different rules for different devices/users
- 🔄 **Reverse proxy ready** — configurable base path (`GS_BASE_PATH`) for running behind Nginx, Caddy, etc.

---

## Quick Start

### Using `docker run`

```bash
docker run -d \
  --name gatesentry \
  --network host \
  --restart unless-stopped \
  -v gatesentry-data:/usr/local/gatesentry/gatesentry \
  -e TZ=America/New_York \
  jbarwick/gatesentry:latest
```

### Using Docker Compose

```yaml
services:
  gatesentry:
    image: jbarwick/gatesentry:latest
    container_name: gatesentry
    restart: unless-stopped
    network_mode: host
    volumes:
      - gatesentry-data:/usr/local/gatesentry/gatesentry
    environment:
      - TZ=America/New_York

volumes:
  gatesentry-data:
```

```bash
docker compose up -d
```

Then open **http://\<host-ip\>** in a browser. Default login: `admin` / `admin`.

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TZ` | `UTC` | Timezone for time-based access rules (e.g., `America/New_York`) |
| `GS_ADMIN_PORT` | `80` | Port for the web admin UI |
| `GS_BASE_PATH` | `/gatesentry` | URL base path prefix (set to `/` for root-level access) |
| `GS_DEBUG_LOGGING` | `false` | Enable verbose debug logging |
| `GS_MAX_SCAN_SIZE_MB` | `10` | Max content size to scan (MB) |
| `GS_TRANSPARENT_PROXY` | `true` | Enable/disable transparent proxy |
| `GS_TRANSPARENT_PROXY_PORT` | auto | Custom port for transparent proxy listener |

### Ports

| Port | Protocol | Service |
|------|----------|---------|
| 53 | UDP + TCP | DNS server (core service) |
| 80 | TCP | Web admin UI |
| 10413 | TCP | HTTP(S) filtering proxy |
| 5353 | UDP | mDNS/Bonjour listener (device discovery) |

### Volumes

| Path | Description |
|------|-------------|
| `/usr/local/gatesentry/gatesentry` | Persistent data — settings DB, device inventory, certificates, logs |

**Back up this volume** to preserve your configuration.

---

## Why `network_mode: host`?

GateSentry needs host networking to see real client IP addresses. Without it, all
devices appear as the Docker bridge IP and per-device filtering/discovery won't work.

| Feature | Needs host networking? |
|---------|----------------------|
| DNS filtering | Recommended |
| See real client IPs | **Yes** |
| Per-device controls | **Yes** |
| mDNS/Bonjour discovery | **Yes** |
| Passive device discovery | **Yes** |

> **Docker Desktop (macOS/Windows):** Host networking maps to the LinuxKit VM, not
> your real LAN. Use bridged mode with explicit port mappings for local testing only.

---

## Reverse Proxy Setup

Set `GS_BASE_PATH` and `GS_ADMIN_PORT` to run behind a reverse proxy:

```yaml
environment:
  - GS_ADMIN_PORT=8080
  - GS_BASE_PATH=/gatesentry   # default — serves at /gatesentry/
```

**Nginx example:**
```nginx
location /gatesentry/ {
    proxy_pass http://127.0.0.1:8080/gatesentry/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

To serve at root instead: `GS_BASE_PATH=/`.

---

## DHCP / DDNS Integration

Point your router's DHCP DNS setting to GateSentry's IP. Devices will start using
GateSentry after their next DHCP lease renewal.

For routers that support RFC 2136 (pfSense, ISC DHCP, Kea), configure DDNS updates
with TSIG authentication so GateSentry automatically learns device hostnames. See the
[full deployment guide](https://github.com/jbarwick/Gatesentry/blob/master/DOCKER_DEPLOYMENT.md)
for router-specific setup instructions.

---

## Fork Enhancements

This image is built from [jbarwick/Gatesentry](https://github.com/jbarwick/Gatesentry), a fork that adds:

- **Docker-first deployment** — optimized Dockerfile, Compose files, and publish pipeline
- **Automatic device discovery** — passive DNS + mDNS/Bonjour + RFC 2136 DDNS
- **Configurable base path** — `GS_BASE_PATH` for reverse proxy / NAS deployments
- **Synology NAS support** — tested with Synology Container Manager
- **Nexus registry support** — push to private registries

Upstream project: [github.com/fifthsegment/Gatesentry](https://github.com/fifthsegment/Gatesentry)

---

## Links

- 📦 **Source**: [github.com/jbarwick/Gatesentry](https://github.com/jbarwick/Gatesentry)
- 📖 **Deployment Guide**: [DOCKER_DEPLOYMENT.md](https://github.com/jbarwick/Gatesentry/blob/master/DOCKER_DEPLOYMENT.md)
- 🐛 **Issues**: [github.com/jbarwick/Gatesentry/issues](https://github.com/jbarwick/Gatesentry/issues)
- 🔀 **Upstream**: [github.com/fifthsegment/Gatesentry](https://github.com/fifthsegment/Gatesentry)
