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

The durable device assignment subset lives in a new `GSDevices` store. It is
written with its own `assignments` key whose payload records
`{"version":1,"assignments":...}`. Discovery owns observed identity at
runtime; only the user-managed subset (ID, DNS name, manual name, owner,
category, hostnames, mDNS names, MACs, first seen, persistent) is durable.

## Upgrade behavior

On startup, `GSSettings` is opened and its schema version is read. Migration 1
converts a legacy bare-array `rules` value into the current `RuleList` object
format and stamps `settings_schema_version`. An already current store is left
byte-identical when no content conversion is required. Missing optional keys
stay missing during migration; runtime default seeding fills them afterwards
without overwriting operator-set values. Credentials, rules, and explicitly
disabled optional features are preserved. `GSDevices` assignments are restored
into the in-memory device store when the DNS server starts, and re-discovered
traffic merges back into the same device by ID.

## Failure and rollback

A schema version newer than this release, an invalid version, malformed JSON,
or corrupted encrypted data fails closed: startup returns an error and the
affected file is left byte-identical, so no partial state is written. Recovery
is to restore the pre-upgrade data directory backup, or run a GateSentry
release that supports the newer schema, then retry.

Rolling back to a pre-`GSDevices` binary from the same encryption generation
is content-compatible for settings: schema version 1 data is readable by
those binaries, and the unknown `settings_schema_version` key is preserved but
ignored. That older binary ignores `GSDevices` entirely, so device assignments
survive only if the assignment store is kept for a later restart of a newer
binary. If instead the pre-upgrade data directory is restored, any assignments
made after the upgrade are lost with the rest of that snapshot's state. Keep
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

A backup from this release restores to any release that understands its schema
versions. Older binaries without content-migration support ignore the schema
version key and the `GSDevices` store rather than failing, so a schema-1
backup is also content-compatible with the immediately preceding releases;
future schema versions will state their compatibility in release notes.
