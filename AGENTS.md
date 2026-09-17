# GateSentry agent instructions

## Project working memory

Read the current project working memory before meaningful implementation, reprioritization, or ticket creation:

https://linear.app/aial/project/project-teyxg7a2-5zt2rzup-081f9d004473

The Linear project content is the canonical handoff for the product objective, repository topology, delivery order, decisions, risks, backlog, and deferred work. Issue descriptions contain detailed implementation contracts. Verify claims against the checked-out repository and tests.

After meaningful work:

1. Record validation commands and outcomes in the relevant Linear issue.
2. Record changed risks, decisions, and follow-up work in the project working memory.
3. Identify the next recommended issue for the next agent.

## Working style

GateSentry is a self-hosted family and small-office internet safety gateway. Work toward this path: install the gateway, protect a device, apply a policy, understand a filtering decision, and recover from a false positive.

- Trivial changes can proceed directly.
- For medium ambiguity, state the key assumption and inspect relevant code before editing.
- For non-trivial work, resolve state ownership, feedback, coupling, timing, and security before implementation.
- If a critical invariant is unclear, stop and flag the gap rather than guessing.

## Codebase topology

- The root Go module uses local workspace modules `application` and `gatesentryproxy`.
- DNS provides blocklists, custom records, passive DNS, mDNS/Bonjour discovery, and DDNS support.
- The proxy supports explicit HTTP/HTTPS and Linux transparent REDIRECT/TPROXY paths.
- HTTPS MITM, domain/URL/keyword/MIME/time rules, users, devices, logs, statistics, and optional AI image filtering are existing areas.
- The Svelte frontend is in `ui/`; built assets are embedded under `application/webserver/frontend/files`.
- Configuration and application state use local file-backed storage.
- Tests include Go, Python, UI, proxy, DNS, integration, and Docker smoke checks.

## Invariants

Before shipping non-trivial work, answer these questions:

- **State:** discovery owns observed device identity; policy evaluation owns group assignment and rule selection; DNS/proxy adapters enforce decisions; storage owns durable configuration.
- **Feedback:** filtering results become structured decisions reused by logs, block pages, APIs, statistics, and diagnostics.
- **Blast radius:** policy changes affect DNS, explicit proxy, transparent proxy, and content inspection. Account for unknown devices, IP churn, NAT/shared identities, stale observations, DNS caching, and persistent connections.
- **Timing:** discovery is asynchronous; schedules need an IANA time zone; overrides need durable expiry; network and AI calls need cancellation and bounded timeouts.
- **Security:** credentials, installation keys, certificates, admin exposure, backups, external AI processing, and authorization require explicit handling and tests.

Do not make device metadata appear to enforce a policy unless it reaches the policy evaluator. Do not claim DNS filtering provides URL, MIME, or content explanations. Do not claim blanket HTTPS, VPN, QUIC, or malware coverage without a verified test matrix.

## Implementation rules

- Inspect the relevant Linear issue, branch/status, this file, and existing tests before editing.
- Commit with the identity configured by this repository. Do not override `user.name`, `user.email`, `GIT_AUTHOR_*`, or `GIT_COMMITTER_*` with `git -c`, environment variables, or `--author`, and do not bypass the identity hook with `--no-verify`.
- Reproduce current behavior for security, persistence, filtering, build, and timing changes.
- Keep state ownership explicit and avoid a second source of truth.
- Propagate storage and network errors; do not ignore write, close, response, or parsing failures on user-critical paths.
- Use atomic persistence and consistent locking for shared configuration.
- Keep credentials, keys, private browsing data, image contents, and stable device identifiers out of logs and telemetry.
- External AI processing is disabled by default, visible in the UI, bounded, and covered by failure-path tests.
- Preserve configuration compatibility through explicit versioning and migrations.
- Keep DNS-first onboarding simple; HTTPS MITM is advanced and opt-in with documented exclusions.

## Validation

Use the narrowest relevant checks while iterating, then all applicable checks before handoff. The intended fast path is `make verify`. If unavailable, run relevant commands directly and record exact commands and outcomes in Linear. Depending on the change, include `go test ./...`, `cd ui && npm run check && npm test -- --run`, `make test`, and `docker build .`.

Do not report a check as passing when it was skipped because a dependency, toolchain, service, or environment was unavailable.

## Delivery order

1. Secure first-run setup, reliable storage/encryption, reproducible builds, migrations, and security documentation.
2. Verified Docker installation and DNS-first onboarding with an end-to-end protection check.
3. Explicit device policy groups, assignment UI, templates, structured decisions, explanations, and scoped exceptions.
4. Schedules, pauses, simulation, backup/restore, diagnostics, understandable HTTPS inspection, and consolidated AI.
5. Customer validation, focused positioning, website/demo, paid installation support, sponsorship, and validated recurring services.

Do not start a hosted control plane, enterprise positioning, hardware appliance, native mobile agent, or paid AI packaging without evidence from validation work.

## Handoff format

When completing an issue, record what changed and why; preserved invariants; tests and commands; configuration, migration, rollout, or rollback implications; limitations; and the next Linear issue. Update the Linear project memory when objectives, risks, decisions, delivery order, or next work changes.
