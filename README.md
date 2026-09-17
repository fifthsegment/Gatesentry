# Gatesentry

HTTP/HTTPS proxy with SSL interception (MITM), content filtering, and a built-in DNS sinkhole. Ships with a web admin dashboard.

[![Codecov](https://codecov.io/gh/fifthsegment/Gatesentry/branch/master/graph/badge.svg)](https://codecov.io/gh/fifthsegment/Gatesentry)
[![Release](https://img.shields.io/github/v/release/fifthsegment/Gatesentry)](https://github.com/fifthsegment/Gatesentry/releases/latest)

[Security policy and vulnerability reporting](SECURITY.md) · [Privacy, AI data handling, backups, and filtering limits](SECURITY.md#local-data-and-privacy)

## Reproducible build

GateSentry builds with Go 1.24.10, Node.js 24.x, and Yarn 4.10.3. Enable the exact Yarn version declared in `ui/package.json` with `corepack enable && corepack prepare yarn@4.10.3 --activate`, then run `make verify`. This fast path performs an immutable dependency install, UI checks and tests, a fresh dashboard build and embedded-asset sync, embedded dashboard and block-page tests, application and proxy tests, and a Go build. Any failed stage stops the build.

Use `make verify-go` when the frontend assets have already been freshly synced and only Go checks are needed. `make docker-smoke` separately builds the checked-out revision into an image and exercises the dashboard and explicit proxy; it requires Docker and network access. The slower privileged integration suite remains available through `make test`.

`make release-artifacts` writes cross-platform binaries, checksums, and the exact source commit to `dist/`. Ordinary branch builds only produce reviewable artifacts. Release publication requires an existing tag that resolves to the checked-out commit, and Docker publication uses the same tagged source rather than downloading another release.

## What it does

Runs as a local proxy on your machine or network. Clients route traffic through it and Gatesentry can:

- Inspect and filter HTTPS traffic when advanced MITM is enabled, routed, and configured with a trusted CA certificate
- Block domains via DNS (runs its own DNS server, pulls blocklists from external sources)
- Match URLs and content against keyword, MIME, and domain rules
- Apply time-based and per-user access schedules
- Log DNS and proxy decisions and display stats in the web UI

Useful as a network-wide content filter, a privacy guard, a parental control layer, or a sinkhole for known-bad domains.

![gatesentry-repo](https://github.com/fifthsegment/Gatesentry/assets/5513549/5ab836ab-7362-4916-9f7c-655e67e4deab)

## Getting started

There are 2 ways to run Gatesentry, either using the docker image or using the single file binary directly.

Fresh installations require one-time administrator setup. See [Secure first-run administration](docs/secure-first-run.md) before exposing the dashboard or configuring unattended Docker startup.

### Method 1: Using Docker

GateSentry runs on a Linux Docker host and publishes `linux/amd64` and `linux/arm64` images. The verified quickstart builds from a repository checkout so you run the exact source you pinned:

```bash
git clone https://github.com/fifthsegment/Gatesentry.git
cd Gatesentry
docker compose up -d --build
docker compose ps   # wait for the gateway to report (healthy)
```

The dashboard is at `http://127.0.0.1:10786` on the Docker host. Fresh installations have no default credentials; create the administrator on the one-time setup screen.

For a copyable quickstart without a checkout, [docker-compose.prebuilt.yml](docker-compose.prebuilt.yml) pulls the published `abdullahi1/gatesentry` image. The currently published image predates the secure first-run release: it still starts with legacy `admin/admin` credentials and lacks the `/health` endpoint, so change that password immediately or prefer the verified build above until the next release is published.

GateSentry uses host networking to serve DNS on port 53 and see real client addresses. Read [Docker installation, upgrade, and rollback](docs/docker.md) before production use: it covers DNS port conflicts (for example `systemd-resolved`), firewall exposure, persistent state and backups, readiness checks and logs, pinned upgrades with rollback, and router DNS configuration.

### Method 2: Using the Gatesentry binary directly

1.  Downloading Gatesentry:

    Navigate to the 'Releases' section of this repository.
    Download the binary for your operating system and CPU:

    | Operating system | x86-64 / amd64 | ARM64 |
    | ---------------- | -------------- | ----- |
    | Linux | `gatesentry-linux-amd64` | `gatesentry-linux-arm64` |
    | macOS | `gatesentry-darwin-amd64` | `gatesentry-darwin-arm64` |
    | Windows | `gatesentry-windows-amd64.exe` | Not currently published |

    Releases currently provide the Windows binary directly; a Windows installer is not published.

2.  Installation:

    **For macOS and Linux:**

    Locate the downloaded Gatesentry binary file in your system.
    Open a terminal window and navigate to the directory containing the downloaded binary.
    Run the following command to grant execution permissions to the binary file:

        chmod +x gatesentry-{os}-{arch}

    Replace `{os}` with `linux` or `darwin` and `{arch}` with `amd64` or `arm64`.
    Proceed to execute the binary file to initiate the server.

    **Running as a Service (Optional)**

    If you want Gatesentry to keep running in the background on your machine, install it as :

    `./gatesentry-{os}-{arch} -service install`

    Next, on linux you can use your system service runner to start or stop it, for example for ubuntu:

    `service gatesentry start   #starts the service`

    `service gatesentry stop    #stops the service`

    **For Windows**

    Download `gatesentry-windows-amd64.exe` and run it from PowerShell or Command Prompt.

    **Running as a Service**

    Run `gatesentry-windows-amd64.exe -service install` from an elevated PowerShell or Command Prompt, then look for GateSentry in the Windows Services manager (`services.msc`).

3.  Start the server:

    ```
    ./gatesentry-{os}-{arch}
    ```

    The proxy listens on port 10413, admin UI on port 10786.

### Run as a background service

**Linux / macOS:**

```
./gatesentry-{os}-{arch} -service install
service gatesentry start
service gatesentry stop
```

**Windows:** Run `gatesentry-windows-amd64.exe -service install` from an elevated shell, then manage GateSentry through `services.msc`.

| Port  | Purpose                        |
| ----- | ------------------------------ |
| 10413 | Explicit proxy (all interfaces) |
| 10414 | Transparent proxy (Linux; optional routing required) |
| 10786 | Plain-HTTP web admin panel (all interfaces) |
| 53    | DNS server (TCP and UDP; all interfaces by default) |

### First-run setup

Fresh installations have no default credentials. Open `http://127.0.0.1:10786` (or `http://[::1]:10786`) on the GateSentry host and create the administrator on the one-time setup screen. Setting up from another machine requires a one-time bootstrap file; see [Secure first-run administration](docs/secure-first-run.md). Docker deployments follow the same flow, including unattended startup with the bootstrap file described in [Docker installation, upgrade, and rollback](docs/docker.md).

Restrict these listeners to trusted clients with the host firewall. Do not
expose the admin UI directly to the Internet. The supplied Docker Compose file
uses host networking, so the host firewall controls its exposure. See the
[security and hardening guidance](SECURITY.md#administrative-exposure-and-hardening).

### DNS

DNS-first onboarding is the simple default. The DNS server blocks domains from
external blocklists. Use `dns_resolver` in settings to choose an upstream
(defaults to `8.8.8.8:53`). DNS filtering acts on domains; it cannot inspect or
explain URL, MIME, keyword, or image-content decisions. See the
[filtering coverage limits](SECURITY.md#filtering-coverage-limits).

## Transparent Proxy Mode (Linux only)

GateSentry automatically enables transparent proxy mode on Linux systems. This allows traffic interception without client configuration using Linux's `SO_ORIGINAL_DST` socket option and `IP_TRANSPARENT` socket support for TPROXY.

### Setup for Local Traffic (REDIRECT mode)

For traffic originating from the local machine:

```bash
iptables -t nat -A PREROUTING -p tcp --dport 80 -j REDIRECT --to-port 10414
iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 10414
```

### Setup for Forwarded Traffic (TPROXY mode)

For traffic forwarded through the machine (e.g., Tailscale exit node, router):

```bash
# Mark traffic for routing
iptables -t mangle -A PREROUTING -p tcp --dport 80 -j TPROXY --tproxy-mark 0x1/0x1 --on-port 10414
iptables -t mangle -A PREROUTING -p tcp --dport 443 -j TPROXY --tproxy-mark 0x1/0x1 --on-port 10414

# Route marked traffic locally
ip rule add fwmark 1 lookup 100
ip route add local 0.0.0.0/0 dev lo table 100
```

### Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `GS_TRANSPARENT_PROXY_PORT` | Port for transparent proxy | `10414` |
| `GS_TRANSPARENT_PROXY` | Set to `false` to disable | `true` on Linux |

### Requirements

- Linux with `SO_ORIGINAL_DST` and `IP_TRANSPARENT` support
- Root or CAP_NET_ADMIN privileges
- CA certificate installed on clients for HTTPS interception

### Features

- Supports both REDIRECT (local) and TPROXY (forwarded) traffic
- Auto-starts on Linux with graceful fallback
- Protocol auto-detection (HTTP vs HTTPS)
- SSL Bump support for HTTPS filtering
- Applies filters supported by the detected proxy path; review the [HTTPS and protocol limitations](SECURITY.md#filtering-coverage-limits)

## Local Development

`./setup.sh`

To run it:

`./run.sh`
