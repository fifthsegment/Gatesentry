# Gatesentry

Filtering proxy and DNS server for home and small networks, with a web dashboard.

[![Codecov](https://codecov.io/gh/fifthsegment/Gatesentry/branch/master/graph/badge.svg)](https://codecov.io/gh/fifthsegment/Gatesentry)
[![Release](https://img.shields.io/github/v/release/fifthsegment/Gatesentry)](https://github.com/fifthsegment/Gatesentry/releases/latest)

[Security policy](SECURITY.md) · [Privacy and data handling](SECURITY.md#local-data-and-privacy) · [Filtering limits](SECURITY.md#filtering-coverage-limits) · [Changelog](CHANGELOG.md)

![gatesentry-repo](https://github.com/fifthsegment/Gatesentry/assets/5513549/5ab836ab-7362-4916-9f7c-655e67e4deab)

## Features

- **DNS filtering** for the whole network, using downloadable blocklist categories
- **Policies per device or proxy user.** Each policy has blocked categories, blocked domains and always-allowed domains, an optional safe search setting, and ordered rules. Devices with no assigned policy use the Default policy.
- **Rules** apply to all traffic, specific domains, or categories. A rule can have a schedule (for example, no internet at bedtime) and can apply only to certain users.
- **Safe search** for Google, Bing, DuckDuckGo, and YouTube restricted mode, enforced through DNS
- **HTTP/HTTPS proxy** (explicit on port 10413, transparent on Linux). With HTTPS inspection enabled and the CA certificate installed on clients, rules can also match URL paths and response content types.
- **Keyword and content filters**, plus optional AI image filtering (off by default; see [SECURITY.md](SECURITY.md#optional-ai-image-filtering))
- **Exceptions and pauses** that lift a block for a set time
- **Test a site** from the policies page to see which rule decides a request
- **Logs, stats, device inventory, backup and restore**

DNS filtering works on domains only. URL, keyword, and content rules need traffic to pass through the proxy. See the [filtering coverage limits](SECURITY.md#filtering-coverage-limits).

## Getting started

Fresh installations have no default credentials. Open the dashboard and create the administrator on the setup screen. Until setup is complete, anyone who can reach the dashboard can claim the admin account, so finish setup before exposing the port. See [Secure first-run administration](docs/secure-first-run.md).

### Docker

Images are published for `linux/amd64` and `linux/arm64`.

```bash
git clone https://github.com/fifthsegment/Gatesentry.git
cd Gatesentry
docker compose up -d --build
docker compose ps   # wait for (healthy)
```

To use the published `abdullahi1/gatesentry` image instead of building from source, use [docker-compose.prebuilt.yml](docker-compose.prebuilt.yml).

The container uses host networking so it can serve DNS on port 53 and see real client addresses. [docs/docker.md](docs/docker.md) covers port 53 conflicts (e.g. `systemd-resolved`), firewalling, persistent state, upgrades and rollback, and pointing your router's DNS at Gatesentry.

### Binary

Download the binary for your platform from [Releases](https://github.com/fifthsegment/Gatesentry/releases/latest):

| OS      | amd64                          | arm64                    |
| ------- | ------------------------------ | ------------------------ |
| Linux   | `gatesentry-linux-amd64`       | `gatesentry-linux-arm64` |
| macOS   | `gatesentry-darwin-amd64`      | `gatesentry-darwin-arm64` |
| Windows | `gatesentry-windows-amd64.exe` | —                        |

```bash
chmod +x gatesentry-linux-amd64
./gatesentry-linux-amd64
```

Then open `http://<host>:10786`.

To run it as a service:

```bash
./gatesentry-linux-amd64 -service install
service gatesentry start
```

On Windows, run `gatesentry-windows-amd64.exe -service install` from an elevated shell and manage it in `services.msc`.

### Ports

| Port  | Purpose |
| ----- | ------- |
| 53    | DNS (TCP and UDP) |
| 10413 | Explicit HTTP/HTTPS proxy |
| 10414 | Transparent proxy (Linux, needs routing rules) |
| 10786 | Web dashboard (plain HTTP) |

All listeners bind to every interface. Restrict them to trusted clients with the host firewall and don't expose the dashboard to the internet. See [hardening guidance](SECURITY.md#administrative-exposure-and-hardening).

### Pointing clients at Gatesentry

The simplest setup is to set Gatesentry as the DNS server on your router or on each device. The dashboard home page walks through this and runs an end-to-end check; see [docs/onboarding.md](docs/onboarding.md).

For URL and content filtering, set Gatesentry as the device's HTTP proxy (`<host>:10413`), or use transparent mode on Linux (below). To inspect HTTPS, turn on HTTPS filtering in settings and install the CA certificate from the dashboard on each client.

## Configuration

Most settings are in the dashboard. These environment variables are read at startup:

| Variable | Description | Default |
| -------- | ----------- | ------- |
| `GATESENTRY_DNS_PORT` | DNS listen port | `53` |
| `GATESENTRY_DNS_RESOLVER` | Upstream DNS resolver | `8.8.8.8:53` |
| `GS_TRANSPARENT_PROXY` | Set to `false` to disable the transparent proxy | `true` on Linux |
| `GS_TRANSPARENT_PROXY_PORT` | Transparent proxy port | `10414` |
| `GS_MAX_SCAN_SIZE_MB` | Largest response body scanned by content filters, in MB (max 1000) | `10` |
| `GS_DEBUG_LOGGING` | Set to `true` for verbose logs | `false` |
| `GATESENTRY_BOOTSTRAP_FILE` | Admin credentials file for unattended first start (see [docs/secure-first-run.md](docs/secure-first-run.md)) | |

## Transparent proxy (Linux)

The transparent proxy on port 10414 accepts redirected traffic without any client configuration. It needs root or `CAP_NET_ADMIN`.

Traffic from the machine itself (REDIRECT):

```bash
iptables -t nat -A PREROUTING -p tcp --dport 80 -j REDIRECT --to-port 10414
iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 10414
```

Traffic routed through the machine, e.g. a router or Tailscale exit node (TPROXY):

```bash
iptables -t mangle -A PREROUTING -p tcp --dport 80 -j TPROXY --tproxy-mark 0x1/0x1 --on-port 10414
iptables -t mangle -A PREROUTING -p tcp --dport 443 -j TPROXY --tproxy-mark 0x1/0x1 --on-port 10414
ip rule add fwmark 1 lookup 100
ip route add local 0.0.0.0/0 dev lo table 100
```

HTTPS traffic in transparent mode is tunnelled unless HTTPS filtering is on and clients trust the Gatesentry CA.

## Development

Requires Go 1.24.10, Node.js 24, and Yarn 4.10.3 (`corepack enable && corepack prepare yarn@4.10.3 --activate`).

```bash
./run.sh          # build the UI and binary, then start on port 53
make verify       # UI checks and tests, asset build, Go tests, Go build
make verify-go    # Go checks only
make proxy-smoke  # end-to-end proxy test against a local build
make docker-smoke # build the Docker image and smoke test it
make test         # full integration suite (privileged)
```

## Releases

Bump `GATESENTRY_VERSION` in `main.go`, add a matching section to `CHANGELOG.md`, and merge to `master`. CI builds and tests the commit, tags it `v<version>`, publishes a GitHub release with the changelog section as notes, and pushes the Docker image.
