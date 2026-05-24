# Gatesentry

HTTP/HTTPS proxy with SSL interception (MITM), content filtering, and a built-in DNS sinkhole. Ships with a web admin dashboard.

[![Codecov](https://codecov.io/gh/fifthsegment/Gatesentry/branch/master/graph/badge.svg)](https://codecov.io/gh/fifthsegment/Gatesentry)
[![Release](https://img.shields.io/github/v/release/fifthsegment/Gatesentry)](https://github.com/fifthsegment/Gatesentry/releases/latest)

## What it does

Runs as a local proxy on your machine or network. Clients route traffic through it and Gatesentry can:

- Inspect and filter HTTPS traffic (MITM with a generated CA certificate)
- Block domains via DNS (runs its own DNS server, pulls blocklists from external sources)
- Match URLs and content against keyword, MIME, and domain rules
- Apply time-based and per-user access schedules
- Log all traffic and display stats in the web UI

Useful as a network-wide content filter, a privacy guard, a parental control layer, or a sinkhole for known-bad domains.

![gatesentry-repo](https://github.com/fifthsegment/Gatesentry/assets/5513549/5ab836ab-7362-4916-9f7c-655e67e4deab)

## Getting started

There are 2 ways to run Gatesentry, either using the docker image or using the single file binary directly.

### Method 1: Using Docker

1. Use the [docker-compose.yml](https://github.com/fifthsegment/Gatesentry/blob/master/docker-compose.yml) file from the root of this repo as a template, copy and paste it to any directory on your computer, then run the following command in a terminal `docker compose up`

### Method 2: Using the Gatesentry binary directly

1.  Downloading Gatesentry:

    Navigate to the 'Releases' section of this repository.
    Identify and download the appropriate file for your operating system, named either gatesentry-linux or gatesentry-mac.

2.  Installation:

    **For macOS and Linux:**

    Locate the downloaded Gatesentry binary file in your system.
    Open a terminal window and navigate to the directory containing the downloaded binary.
    Run the following command to grant execution permissions to the binary file:

        chmod +x gatesentry-{platform}

    Replace `{platform}` with your operating system (linux or mac).
    Proceed to execute the binary file to initiate the server.

    **Running as a Service (Optional)**

    If you want Gatesentry to keep running in the background on your machine, install it as :

    `./gatesentry-{platform} -service install`

    Next, on linux you can use your system service runner to start or stop it, for example for ubuntu:

    `service gatesentry start   #starts the service`

    `service gatesentry stop    #stops the service`

    **For Windows**

    The installer (GatesentrySetup.exe) contains instructions.

    **Running as a Service**

    The installer (GatesentrySetup.exe) should automatically install a service. You can look for it by searching for gatesentry in your Service manager (open it by running `services.msc`)

3.  Start the server:

    ```
    ./gatesentry-{platform}
    ```

    The proxy listens on port 10413, admin UI on port 10786.

### Run as a background service

**Linux / macOS:**

```
./gatesentry-{platform} -service install
service gatesentry start
service gatesentry stop
```

**Windows:** Run `GatesentrySetup.exe`. The installer registers a Windows service automatically.

| Port  | Purpose                        |
| ----- | ------------------------------ |
| 10413 | Explicit proxy                 |
| 10414 | Transparent proxy (optional)   |
| 10786 | Web admin panel                |
| 53    | DNS server                     |
| 80    | Block page server              |

### Default credentials

```
Username: admin
Password: admin
```

Change the password after first login.

### DNS

The DNS server blocks domains from external blocklists. Use `dns_resolver` in settings to choose an upstream (defaults to `8.8.8.8:53`).

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
- All existing filters work in transparent mode

## Local Development

`./setup.sh`

To run it:

`./run.sh`

## Build instructions
Refering to the build instructions from https://github.com/fifthsegment/Gatesentry/blob/master/bitbucket-pipelines.yml:

- [Install `go`](https://go.dev/doc/install)
- Execute the following commands one by one:
```
git clone https://github.com/fifthsegment/Gatesentry.git # Clones the master repository – change it if you want another branch
cd Gatesentry
mkdir bin
go get -v
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.38.0/install.sh | bash
# You should BETTER use the versions of the following two commands given by the previous command as they might differ
export NVM_DIR="$HOME/.nvm"
"[ -s \"$NVM_DIR/nvm.sh\" ] && \\. \"$NVM_DIR/nvm.sh\""   # This loads nvm
nvm install 18.17 # Install Node.js version 18.17
cd ui && npm install && npm run build && cd ..
rm -rf application/webserver/frontend/files/*
mv ui/dist/* application/webserver/frontend/files
mv application/webserver/frontend/files/fs/* application/webserver/frontend/files
env GOOS=linux GOARCH=amd64 go build
# If you build for macOS uncomment this
# env GOOS=darwin GOARCH=amd64 go build -o gatesentry-macos
# If you build for ARM64 uncomment this
# env GOOS=darwin GOARCH=arm64 go build
mv gatesentrybin gatesentry-linux
```
