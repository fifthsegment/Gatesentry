# Docker installation, upgrade, and rollback

This guide is the verified path for running GateSentry with Docker on a Linux
host. It covers two installation options, networking and DNS port conflicts,
persistent state, readiness checks, logs, pinned upgrades with rollback,
router DNS configuration, and optional certificates.

## Prerequisites

- A Linux Docker host with Docker Engine and the Compose v2 plugin. Host
  networking is a Linux feature; on Docker Desktop (macOS/Windows) the
  container cannot bind LAN-facing DNS reliably, so use a Linux machine or VM.
- Published images support `linux/amd64` and `linux/arm64` (for example
  64-bit Raspberry Pi). Windows containers are not published.
- Free UDP and TCP port 53 for DNS, TCP 10413 for the explicit proxy, and TCP
  10786 for the web dashboard. Port 10414 is used by the optional Linux
  transparent proxy.
- For LAN-wide protection, a fixed LAN IP for the Docker host (static lease or
  reservation) so clients and the router can point DNS at it.

## Option A (verified): build from a repository checkout

This path pins the exact source you run and is the one exercised by CI on a
clean host:

```bash
git clone https://github.com/fifthsegment/Gatesentry.git
cd Gatesentry
docker compose up -d --build
docker compose ps        # wait for the gateway to report (healthy)
```

The [docker-compose.yml](../docker-compose.yml) file builds the image from the
checked-out commit and labels it with the revision. Persistent state is
written under `./docker_root` next to the compose file.

## Option B: published image quickstart

Copy [docker-compose.prebuilt.yml](../docker-compose.prebuilt.yml) to any
directory on the Docker host and run:

```bash
mkdir -p gatesentry-data
docker compose -f docker-compose.prebuilt.yml up -d
```

The image is published as `abdullahi1/gatesentry` for `linux/amd64` and
`linux/arm64`. Pin a specific tag or digest (`image:
abdullahi1/gatesentry:<tag>@sha256:<digest>`) for reproducible deployments.

**Security note on the currently published image:** the latest published
image predates the secure first-run release. It still starts with legacy
`admin/admin` credentials, and it does not yet include the `/health` endpoint
(the compose file probes the dashboard URL for that reason). Log in
immediately and change the administrator password, or prefer Option A until a
release including secure first-run is published.

## First-run setup

Open `http://127.0.0.1:10786` (or `http://[::1]:10786`) in a browser on the
Docker host and create the administrator on the one-time setup screen. Fresh
installations built from current source have no default credentials. Setup is
not authenticated, so complete it before the port is reachable from an
untrusted network; see [Secure first-run administration](secure-first-run.md).
To skip the screen entirely, mount the credentials file shown in the compose
file and remove the mount after setup completes.

## Host networking and DNS port conflicts

GateSentry uses `network_mode: host` because DNS filtering needs UDP/TCP 53
on the LAN and the gateway needs to see real client addresses. Consequences:

- Container ports bind directly on the host, so the host firewall controls
  exposure. Restrict 10786 (dashboard) and 10413 (proxy) to trusted clients.
- Only one DNS server can bind port 53. On Ubuntu/Debian,
  `systemd-resolved` holds it by default. Either free the port:

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
printf '[Resolve]\nDNSStubListener=no\n' | sudo tee /etc/systemd/resolved.conf.d/no-stub.conf
sudo ln -sf /run/systemd/resolve/resolv.conf /etc/resolv.conf
sudo systemctl restart systemd-resolved
```

  or move GateSentry DNS with `GATESENTRY_DNS_PORT` (and optionally
  `GATESENTRY_DNS_ADDR`) in the compose `environment` section. Routers
  normally forward only port 53, so a non-standard port is mostly useful for
  testing.

## Persistent state and backups

All durable state lives in the mounted data directory (`./docker_root` for
Option A, `./gatesentry-data` for Option B), including:

- `installation.key` - per-installation encryption key. Without it the
  settings stores cannot be decrypted; keep it inside every backup.
- `GSSettings`, `GSWebSettings` - encrypted settings stores.
- `GSDevices` - durable device records (stable identifiers).

Back up while the container is stopped so files are consistent:

```bash
docker compose down            # or: docker compose -f docker-compose.prebuilt.yml down
tar -C . -czf gatesentry-backup-$(date +%Y%m%d).tgz docker_root   # or gatesentry-data
docker compose up -d
```

See [Encrypted settings storage](settings-encryption.md) and
[Configuration migrations](configuration-migrations.md) for what each store
contains and how upgrades migrate it.

## Readiness checks and logs

Images built from current source expose an unauthenticated readiness endpoint
that returns `200` with `{"status":"ok"}` and no configuration material:

```bash
curl http://127.0.0.1:10786/health
docker compose ps           # healthcheck state is shown per service
docker inspect --format '{{.State.Health.Status}}' gatesentry
```

The Dockerfile and compose files probe this endpoint, so an unhealthy gateway
is visible in `docker compose ps` after the start period.

Inspect logs with:

```bash
docker compose logs --follow gatesentry
```

Logs may contain visited domains and blocked URLs (private browsing data), so
share them selectively. Administrator credentials, unattended bootstrap
credentials, and encryption keys are never written to the logs. Avoid pasting
`docker inspect` output, which can include environment details, into public
reports.

## Pinned versions, upgrade, and rollback

1. Back up the data directory as shown above.
2. Pin the target version. For Option B edit the `image:` line to a published
   tag or digest. For Option A check out the exact tag or commit and rebuild;
   the image label `org.opencontainers.image.revision` records the source.
3. Upgrade:

```bash
docker compose -f docker-compose.prebuilt.yml pull && \
  docker compose -f docker-compose.prebuilt.yml up -d
# or, for Option A:
git fetch --tags && git checkout <tag> && docker compose up -d --build
```

Settings stores carry an explicit schema version and are content-compatible
with recent releases; migrations run on startup and abort without modifying
existing data if they fail.

To roll back, restore the pre-upgrade data directory and run the previous
image version:

```bash
docker compose down
rm -rf docker_root && tar -xzf gatesentry-backup-<date>.tgz
git checkout <previous-tag> && docker compose up -d --build   # or pin the previous image tag
```

Do not roll binaries back without restoring the matching data-directory
backup: releases older than the secure first-run change cannot use the new
administrator record, and older releases simply ignore newer device records.
See [Secure first-run administration](secure-first-run.md#upgrade-rollout-and-recovery).

## Point your network at GateSentry

Once the gateway answers on the host's LAN IP:

```bash
dig @<gateway-lan-ip> example.com
```

configure clients to use it for DNS, in one of two ways:

- Router: set the WAN or LAN DNS server (often under DHCP settings) to the
  gateway's LAN IP so every DHCP client is protected.
- Single device: set that device's DNS server to the gateway's LAN IP.

DNS filtering acts on domains; it cannot inspect or explain URL, MIME,
keyword, or image-content decisions. See the
[filtering coverage limits](../SECURITY.md#filtering-coverage-limits).

## Optional certificates and HTTPS inspection

DNS-first operation needs no certificates. HTTPS inspection is advanced and
opt-in: generate the GateSentry CA in the dashboard, install it in client
trust stores, and review exclusions and limits in
[Certificate trust and HTTPS interception](../SECURITY.md#certificate-trust-and-https-interception)
before enabling it.
