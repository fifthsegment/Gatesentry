# Security and privacy

This document describes the security support policy and the behavior of the current GateSentry code. GateSentry handles sensitive network data and can act as a certificate authority. Operators should review this document before placing it on a network.

## Supported versions

| Version | Security support |
| --- | --- |
| Latest published release | Supported on a best-effort basis |
| Older releases | Not supported; upgrade before requesting a fix |
| Unreleased default branch | Evaluated on a best-effort basis; not recommended for production |

There is no long-term-support release line. A fix may require upgrading to a newer release.

## Report a vulnerability

GitHub private vulnerability reporting is not enabled for this repository as of 2026-09-14, and the project does not publish a private security email address. Open a [GitHub issue](https://github.com/fifthsegment/Gatesentry/issues/new) containing only a minimal notice that you have a potential security report and how the maintainers can reach you through the contact information on your GitHub profile. A maintainer will arrange a private follow-up channel.

Do **not** put exploit instructions, proof-of-concept code, credentials, API keys, certificate private keys, private browsing data, image contents, or stable device identifiers such as MAC addresses in the public issue. Redact URLs, domains, IP addresses, host names, user names, and logs unless the maintainer has confirmed a private channel and they are needed to assess the report.

The project aims to:

- acknowledge the minimal report within 3 business days;
- provide an initial assessment within 7 business days after receiving enough information privately;
- provide an update at least every 14 days while remediation is in progress; and
- target remediation and coordinated disclosure within 90 days.

These are best-effort targets for a maintainer-run project. Complexity, maintainer availability, and coordination with dependencies may change the schedule. The maintainer will communicate material schedule changes through the private channel. Please allow time for a fix and release before public disclosure.

## Administrative exposure and hardening

The defaults are intended for a trusted local network and are not hardened for direct Internet exposure.

| Service | Current default behavior |
| --- | --- |
| Admin UI/API | Plain HTTP on all interfaces, TCP port `10786` |
| Explicit HTTP/HTTPS proxy | All interfaces, TCP port `10413` |
| Transparent proxy on Linux | Startup is attempted on TCP port `10414` unless `GS_TRANSPARENT_PROXY=false`; interception also requires operator-supplied REDIRECT/TPROXY routing |
| DNS | Enabled by default on UDP and TCP `0.0.0.0:53`; the address and port can be changed with `GATESENTRY_DNS_ADDR` and `GATESENTRY_DNS_PORT` |

The initial admin credentials are `admin` / `admin`. Change them immediately. The application does not add TLS to the admin UI. Restrict all listeners with a host firewall to the clients that need them, place administration on a trusted management network, and do not expose the backend directly to the Internet. A carefully configured reverse proxy can add TLS, authentication, and access controls, but it does not change the plaintext backend listener. Network restrictions remain necessary because the current API includes some routes that do not consistently enforce the same authentication checks.

The supplied [Docker Compose configuration](docker-compose.yml) uses host networking. The container therefore follows the host's listener and firewall exposure; Docker does not isolate the ports behind a published-port list. `EXPOSE` entries in the [Dockerfile](Dockerfile) are informational.

DNS-first onboarding is the simple default. Point only the intended clients at GateSentry's DNS service and verify domain blocking before enabling more complex interception. HTTPS MITM is disabled by default and is an advanced, opt-in deployment that requires client and routing changes.

## Certificate trust and HTTPS interception

GateSentry settings include a CA certificate and its private key. The current default settings are initialized with a certificate/key pair embedded in the source rather than a newly generated, installation-unique authority. Treat that default as a security limitation and replace it with an operator-controlled pair before trusting it on clients where practical.

Installing the configured CA certificate in a client's trust store allows anyone holding the matching private key to create certificates that the client will trust. Restrict the data directory and every backup containing settings, protect the CA key as a high-value secret, and remove the CA from clients when the deployment is retired or compromised.

Trust alone does not route traffic through GateSentry. HTTPS inspection requires HTTPS filtering to be enabled, TCP traffic to reach the explicit proxy or a correctly configured Linux transparent REDIRECT/TPROXY path, and the client to trust the configured CA and accept the forged leaf certificate.

When HTTPS filtering is disabled, or a host is configured not to be bumped, the proxy can tunnel the connection without inspecting its encrypted content. Certificate-pinned applications and clients with incompatible trust behavior can reject interception. Some interception failures in the current proxy can fall back to a direct connection. These cases can fail at the client or bypass content inspection depending on the path; test each important client. GateSentry does not provide universal HTTPS or malware coverage.

## Local data and privacy

For a binary installation, startup builds the data path as a `gatesentry` subdirectory of the directory component used to invoke the program, then converts that path to an absolute path. For the common `./gatesentry-{os}-{arch}` invocation, this is `./gatesentry` under the current working directory. A bare command found through `PATH` also uses the current working directory; the program does not resolve the executable's installed location. With the supplied Docker configuration, `./gatesentry-bin` is launched from `/usr/local/gatesentry`, so the data directory is `/usr/local/gatesentry/gatesentry` in the container and is mounted from `./docker_root` on the host.

The directory can contain:

- `GSSettings`, including admin credentials, users, policy settings, external AI API keys and URLs, DNS configuration, and the CA certificate/private key;
- `GSWebSettings` and the unencrypted `GSUpdateLog`;
- `log.db` by default, containing proxy and DNS activity;
- `filterfiles/*.json`, containing filter configuration; and
- supporting files such as `zoneinfo.zip` when created by the application.

Proxy log records include a timestamp, a user or client identity in the `ip` field, the full requested URL, and the proxy decision. DNS records include a timestamp, queried domain, a DNS/DDNS label in the `ip` field, and the response type. Discovery may also hold stable device data such as UUIDs, host and mDNS names, MAC addresses, IP addresses, first/last-seen times, owner, and category. The inspected implementation keeps that discovery inventory in process memory; it does not establish a durable inventory backup format. Application, service, and container console logs may separately expose domains, URLs, addresses, device identifiers, or names.

Entries written to `log.db` expire after a hard-coded seven-day TTL. There is no operator-configurable retention setting or validated secure-erasure flow. The service/container log driver has its own retention and must be configured separately. To discard GateSentry history, stop the service and securely remove or replace `log.db`; doing so also removes the log-derived history and statistics. Restrict filesystem access, limit OS/container log retention, and collect only the activity needed for operation.

Settings, filter files, and other configuration do not have automatic retention or expiration; they remain until the operator changes or removes them. Deleting them can make the installation unusable, so stop the service and take a protected backup before intentional removal.

Settings stores are written atomically with mode `0600`, but current filter directory/file creation can use broader permissions. Verify ownership and permissions on the entire data directory after installation and restore. Settings encryption uses AES-CFB with a fixed key embedded in the repository and does not provide authenticated integrity. Treat it as limited obfuscation, not protection against someone who can read the files or source. There is no key rotation guarantee.

GateSentry does not currently send the browsing and device records described above through its dormant consumption/heartbeat path. This is not a promise about external services configured by an operator. Never include browsing records, stable device information, credentials, keys, or image contents in public reports or telemetry.

## Backup and recovery

There is no validated backup/export/restore workflow, storage migration guarantee, or automated encryption-key/certificate rotation. For the best available filesystem backup, stop GateSentry and copy the complete data directory so the settings and database are consistent. Include `GSSettings`, `GSWebSettings`, filter files, and any other configuration; include `log.db` only if its sensitive history is required. A Docker operator should back up the host's `docker_root` volume directory.

Store backups encrypted with operator-managed tooling, restrict them to the service owner, and preserve restrictive permissions. A copied `GSSettings` also copies credentials, provider keys, and the CA private key and therefore copies the authority to impersonate sites to trusting clients. The fixed application storage key does not make a copied backup safe. Test restoration offline with the exact release before depending on it; compatibility across versions and successful recovery are not guaranteed by the current code.

## Optional AI image filtering

AI image filtering is disabled by default. When enabled, image response bodies from traffic that reaches the applicable proxy filtering path are encoded and sent according to the selected mode:

| Mode | Recipient and data handling |
| --- | --- |
| `grok` | Sends the encoded image as a data URI to xAI at `https://api.x.ai/v1/chat/completions`; the default model is `grok-4.5`. |
| `chatgpt` | Sends the encoded image as a data URI to OpenAI at `https://api.openai.com/v1/chat/completions`; the default model is `gpt-4o-mini`. |
| `local` | Sends the encoded image to the configured Ollama base URL at `/api/chat`; the default model is `llava`. It remains local only when that URL points to infrastructure controlled by the operator. |

For these three modes, the implementation rejects empty images and images over 8 MiB, uses a 60-second HTTP client timeout with 15-second dial and TLS handshake timeouts, limits response bodies to 1 MiB, and requests at most 200 output tokens from xAI and OpenAI. A classification blocks only an NSFW verdict with confidence of at least 50. The configured client connects directly and does not use proxy environment variables. Provider credentials are saved in settings and inherit the fixed-key storage limitation described above.

Older settings can activate a legacy scanner when AI filtering is enabled, the mode is blank or unrecognized, and `ai_scanner_url` is configured. That path sends selected image types of at least 6,000 bytes as multipart data to the configured endpoint; WebP content is converted to JPEG first. It uses the default HTTP client without the explicit request timeout or image/response bounds of the current modes, and can log the requested URL and complete inference response. Use only a trusted endpoint, review service logs, and migrate away from this legacy mode. AI status checks contact the configured provider endpoints but do not include an image.

External providers process image contents under their own terms and retention rules. Enable them only after informing affected users and deciding that this transfer is appropriate. Do not place real images or API keys in examples, diagnostics, or reports.

## Filtering coverage limits

| Traffic | Current coverage and limitation |
| --- | --- |
| DNS | Blocks or allows domain queries seen by GateSentry. A DNS decision cannot see, enforce, or explain a URL path, MIME type, keyword, or image-content decision. Cached answers and clients using another resolver can avoid it. |
| HTTPS over the proxy | URL/content inspection requires successful MITM, routing through the proxy, and client CA trust. Tunnels, no-bump rules, failures, and incompatible clients may pass traffic without content inspection or may fail. |
| Certificate-pinned clients | Commonly reject forged certificates and are not reliably inspectable. Test and explicitly handle these clients. |
| VPNs and other tunnels | Can bypass DNS and proxy enforcement unless the operator independently routes or blocks them. GateSentry does not provide comprehensive VPN/tunnel control. |
| QUIC / HTTP/3 | Uses UDP and is outside the current TCP HTTP/HTTPS interception paths. It may bypass proxy content inspection unless separately disabled or blocked. |
| Encrypted DNS (DoH/DoT) | Can bypass the local DNS listener when clients can reach an external encrypted resolver. GateSentry does not comprehensively detect or block encrypted DNS. |

Coverage depends on network topology, client configuration, DNS caches, active connections, and the selected policy. Validate DNS, explicit proxy, transparent proxy, exceptions, and failure behavior on the actual network.

## Implementation references

The statements above are based on the current implementations of defaults and data paths in [application/runtime.go](application/runtime.go), [main.go](main.go), [storage](application/storage/storage.go), [logging](application/logger/logger.go), [AI vision filtering](application/filters/ai_vision.go), [legacy AI filtering](application/filters/filter-images-ai.go), [AI mode compatibility](application/filters/ai_mode.go), [web serving](application/webserver/webserver.go), [DNS serving](application/dns/server/server.go), and [proxy interception](gatesentryproxy/ssl.go).
