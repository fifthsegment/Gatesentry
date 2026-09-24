# Configuration migrations

GateSentry now versions the content of its persisted settings and device
assignment data so upgrades preserve policy and device state.

## Ownership and versioning

`GSSettings` content has an explicit schema version stored under the
`settings_schema_version` key. Version 1 is the first versioned format. The
application runtime owns these content migrations: it applies them once, in
order, after opening the store and before any default value is seeded. Storage
remains the owner of envelope and encryption formats, including the
authenticated AES-256-GCM migration described in
[Settings encryption](settings-encryption.md).

The durable device assignment subset lives in `GSDevices`. It is written with
its own `assignments` key whose payload records
`{"version":2,"assignments":...}`. Version 2 adds explicitly linked stable
Tailscale node IDs and their names. Discovery still owns observed identity at
runtime: Tailscale addresses, online state, last-seen time, and Wake-on-LAN MAC
metadata are intentionally omitted and refreshed from `tailscaled`. The other
durable fields are ID, DNS name, manual name, owner, category, hostnames, mDNS
names, LAN MACs, first seen, and persistent.

## Upgrade behavior

On startup, `GSSettings` is opened and its schema version is read. Migration 1
converts a legacy bare-array `rules` value into the current `RuleList` object
format and stamps `settings_schema_version`. An already current store is left
byte-identical when no content conversion is required. Missing optional keys
stay missing during migration; runtime default seeding fills them afterwards
without overwriting operator-set values. Credentials, rules, and explicitly
disabled optional features are preserved. `GSDevices` assignments are restored
into the in-memory device store when the DNS server starts, and re-discovered
traffic merges back into the same device by ID. Version 1 device assignments
are accepted and migrated to version 2 on startup; a future assignment version
is rejected before state is changed.

A restored stable Tailscale link remains attached even while the integration is
disabled or `tailscaled` is unavailable. Once LocalAPI access returns, current
Tailscale addresses are repopulated and resolve to the linked inventory device,
so traffic uses that device's existing policy-group assignment. Peer suggestions
never create links automatically, and unlinked peers do not inherit a device
policy. Because Tailscale does not normally expose hardware MAC addresses,
Wake-on-LAN MAC metadata is informational and never participates in matching.

### Policy groups

Explicit policy groups live in an optional `policy_groups` key in
`GSSettings`, written as `{"version":1,"groups":[...],"assignments":[...]}`.
The key is optional: a missing or empty value means no groups exist and every
adapter keeps its pre-policy behavior (global blocklist and user rules
unchanged). The first start with legacy device owner/category metadata writes
the key exactly once, converting each distinct category (owner as fallback)
into a group with action `none` (informational only, no behavior change) and
stable device assignments. If the key already exists, migration is skipped so
API edits are never overwritten; a failed write leaves the key missing and the
previous default behavior in place. Removing the key rolls back to the
pre-policy behavior (existing user-based rules keep working in both cases).

DNS enforcement applies group decisions by domain only. A group domain
decision cannot evaluate URL-path, MIME-type, or HTTPS-inspection conditions;
the DNS adapter logs and the preview endpoint reports these as
inapplicable conditions rather than silently treating them as enforced. Those
conditions remain enforceable on the explicit and transparent proxy paths
where full requests are available.

## Failure and rollback

A schema version newer than this release, an invalid version, malformed JSON,
or corrupted encrypted data fails closed: startup returns an error and the
affected file is left byte-identical, so no partial state is written. Recovery
is to restore the pre-upgrade data directory backup, or run a GateSentry
release that supports the newer schema, then retry.

A binary that supports only device-assignment version 1 rejects version 2; it
cannot safely read records containing Tailscale links. Roll back by restoring
the complete pre-upgrade backup, not by editing the version marker. A binary
from before `GSDevices` existed ignores the store entirely, so assignments and
Tailscale links survive only if the store is kept for a later restart of a newer
binary. Settings remain content-compatible with those binaries from the same
encryption generation: schema version 1 data is readable and the unknown
`settings_schema_version` key is preserved but ignored. If instead the
pre-upgrade data directory is restored, any assignments or Tailscale links made
after the upgrade are lost with the rest of that snapshot's state. Keep
the pre-upgrade backup until the upgrade has been validated. To roll back,
stop GateSentry, restore the complete data directory from that backup (which
includes the matching `installation.key`), and start the older binary.
Restoring a store written with a schema version newer than the older binary
supports is not supported for a binary that checks versions; keep the backup
until compatibility is confirmed.

## Backup compatibility

Stop GateSentry and back up the complete data directory as one unit. The backup
must include `installation.key`, `GSSettings`, `GSWebSettings`, and now
`GSDevices`. `GSDevices` contains stable device identifiers including MAC
addresses, so protect it as sensitive. Without the matching installation key,
authenticated stores are unrecoverable by design. See
[Settings encryption](settings-encryption.md) for key handling, rotation, and
recovery detail.

A backup from this release restores to releases that understand settings
schema 1 and device-assignment version 2. Assignment version 2 preserves stable
Tailscale node links but does not contain transient Tailscale addresses, online
state, last-seen observations, or Wake-on-LAN MAC metadata. Those values return
after `tailscaled` is reachable and the integration refreshes.

A version-1 assignment backup remains accepted and is migrated to version 2. A
future assignment version is rejected. Older binaries without a `GSDevices`
reader ignore the store rather than failing, while binaries that explicitly
validate only assignment version 1 reject version 2. For recovery or rollback,
stop GateSentry and restore the complete matching data-directory backup,
including `installation.key`; never lower the embedded version number by hand.
