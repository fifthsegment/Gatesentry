# Guided DNS-first onboarding and protection check

This guide defines the pilot protocol for DNS-first onboarding: how an
operator or family member gets from a fresh GateSentry installation to a
first explained filtering decision, and how that time is measured. The goal
is a target of ten minutes, recorded per pilot session; the measurement
method is defined here, and pilot data is recorded separately.

The in-product flow lives on the dashboard home page. It is intentionally
DNS-first: no proxy configuration or HTTPS inspection is required to reach a
first explained protection.

## What the guided flow covers

The dashboard home page presents the steps in order and persists progress so
a half-finished setup survives a page reload or restart:

1. **Choose a resolver** (upstream DNS) for non-blocked queries.
2. **Check blocklist readiness** before relying on filtering.
3. **Point a client or the router** at GateSentry for DNS.
4. **Identify the first device** by browsing briefly so discovery observes
   its DNS traffic.
5. **Run the protection check**, which produces an actual, explained
   filtering decision.

Progress, failures, and the timestamp of the first explained protection are
persisted server-side under the `onboarding_progress` settings key (schema
version 1; see [configuration migrations](configuration-migrations.md)).
Only the controlled test domain and its decision are recorded; no browsing
history is stored.

## Actionable failures

The `/api/onboarding/status` endpoint reports port, upstream resolver, and
blocklist readiness separately, each with an actionable next step:

- **Port failure** (GateSentry cannot answer DNS where you expected, for
  example `systemd-resolved` holds port 53): free the port or set
  `GATESENTRY_DNS_ADDR`/`GATESENTRY_DNS_PORT`, then retry.
- **Upstream failure** (GateSentry answers but cannot reach its resolver):
  choose a reachable upstream (`dns_resolver` in settings) and retry.
- **Blocklist failure** (no domains loaded): refresh or wait for blocklists
  to finish downloading, then retry.

Failures are reported per check rather than as a single "not working"
status, so the operator can act on the specific broken link.

## The protection check

Server startup alone is not protection. The protection check asks the running
GateSentry DNS server to resolve the controlled test domain
`gatesentry-protection-check.invalid` and records the decision it actually
made:

- **Blocked** is expected and passes the check: the answer is NXDOMAIN with a
  `blocked.local.` CNAME, exactly the sinkhole behavior a real blocked domain
  receives.
- **Servfail or no answer** fails the check and reports a resolver or
  listener failure, with the actionable hint above.

The test domain is reserved (`.invalid`, RFC 6761) and cannot collide with a
real blocklist entry. The check stores only the test domain label and its
decision, plus timestamps, in `onboarding_progress`.

## Verified coverage and what is not verified

DNS-first onboarding verifies DNS filtering for devices that use GateSentry
as their DNS server. It does not verify per-device proxy use, and it cannot
inspect or explain URL, MIME, keyword, or image-content decisions (see
[filtering coverage limits](../SECURITY.md#filtering-coverage-limits)).

HTTPS inspection is optional and advanced. Enabling it does not change the
onboarding steps above and does not extend the protection check; devices and
protocols that do not use GateSentry for HTTPS remain outside HTTPS
inspection.

For consent, platform-specific certificate install/remove steps, a
verification procedure, exclusions, and limits, see
[HTTPS inspection](https-inspection.md).

## Ten-minute pilot protocol

Run this script with pilot operators who have not seen the product before.
Record clock times at each mark; the measured metric is the elapsed time from
the start of step 1 to the first passing protection check with a persisted
explanation.

1. **Start the clock** when the operator opens the dashboard home page after
   first-run setup. Note `started_at` from `onboarding_progress`.
2. The operator completes resolver selection and blocklist readiness checks
   (steps 1-2), resolving any reported port/upstream/blocklist failures.
3. The operator points a client (or the router) at GateSentry (step 3).
4. The operator browses briefly on the client so the first device appears
   (step 4).
5. The operator runs the protection check (step 5) and reads the explained
   decision.
6. **Stop the clock** when the check passes and the decision is displayed.
   Note `first_explained_protection_at` from `onboarding_progress`; the
   elapsed time is the pilot measurement.

A session that ends without a passing check records the last failed action
instead. Record pilot sessions (date, participant role, elapsed time, and
where time was lost) in the product working memory so the target can be
assessed against evidence rather than assertion.
