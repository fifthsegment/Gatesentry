# Settings encryption

GateSentry encrypts the `GSSettings`, `GSWebSettings`, and `GSDevices`
stores. Other stores remain plaintext unless their caller explicitly opts in to encryption.

## Formats and automatic migration

Older releases wrote a JSON envelope with `data` and `encrypted` string fields.
For encrypted files, `data` was unpadded base64url containing an AES-256-CFB IV
and ciphertext. The encryption key was embedded in every GateSentry binary and
the padding was not authenticated. That legacy format provides no meaningful
protection from a reader of the binary or source.

New encrypted envelopes retain those fields and add
`"version":"aes-256-gcm-v1"`. Their `data` field starts with the `gcm1:` format
marker followed by unpadded base64url containing a fresh random nonce,
AES-256-GCM ciphertext, and its authentication tag. The marker also prevents a
new ciphertext from being mistaken for the unversioned legacy format.
The authenticated additional data includes the format version and store name,
so moving ciphertext between stores is rejected. GateSentry refuses unknown
versions and ciphertext that does not authenticate.

At first successful load, a legacy encrypted file is decrypted with the legacy
reader and replaced through the atomic storage write path with the authenticated
format. The legacy key is never used for new writes. A malformed legacy file,
an invalid GCM file, an unavailable key, or a pre-commit migration failure
returns an error and leaves the settings file unchanged. Do not delete a file
that fails migration; restore its matching key and data from backup, correct
permissions, or recover the original file before restarting.

## Installation key

On the first encrypted write or legacy migration, GateSentry creates
`installation.key` in the data directory. It initially contains 32 random bytes
and is atomically installed with mode `0600`. After a key rotation it is a
versioned JSON keyring containing base64-encoded current and retained previous
32-byte keys. The keyring remains mode `0600`. Key bytes are never printed by
the storage API.

Set `GATESENTRY_INSTALLATION_KEY_FILE` to an absolute path to use an existing
operator-managed key file instead. The file may contain exactly 32 raw bytes or
a GateSentry keyring produced by rotation. It must be a regular file with no
group or other permission bits (normally `0600` or read-only `0400`). GateSentry
returns an error for a missing, unreadable, malformed, or over-permissive
managed file and never silently creates a replacement. Rotation needs write
access to the managed file and its parent directory; ordinary operation only
needs read access.

If authenticated settings already exist and the default `installation.key` is
missing, GateSentry does not generate a new one. A new key could never decrypt
those settings, so startup fails with recovery guidance instead.

## Backup and recovery

Stop GateSentry and back up the complete data directory as one unit. The backup
must include `installation.key`, `GSSettings`, `GSWebSettings`, and `GSDevices`. Without the
matching current or retained previous key, authenticated settings are
unrecoverable by design. Protect the key and backups as secrets: settings can
contain administrator credentials, provider keys, and the HTTPS interception
CA private key, while `GSDevices` contains stable device identifiers
including MAC addresses.

When `GATESENTRY_INSTALLATION_KEY_FILE` points outside the data directory, back
up that managed file separately with the same snapshot. Docker deployments must
keep `installation.key` inside the persistent data volume (the supplied setup
mounts the host `docker_root` directory) and include it in volume backups.
Restore the key/keyring and encrypted stores together, preserve restrictive
ownership and permissions, and test the restored copy offline before relying
on it.

## Key rotation

Rotation is an offline storage operation exposed by the storage package:

```go
err := gatesentry2storage.RotateInstallationKey("GSSettings", "GSWebSettings", "GSDevices")
```

Use this sequence in an operator tool or maintenance build:

1. Stop every GateSentry process that uses the data directory.
2. Take a protected backup of the key file and every encrypted store.
3. Call `RotateInstallationKey` with every encrypted store name.
4. Start GateSentry and verify settings, device assignments, and normal operation.
5. Take and test a new backup.
6. In a later offline maintenance window, call
   `PrunePreviousInstallationKeys("GSSettings", "GSWebSettings", "GSDevices")`.

Rotation first atomically installs a keyring containing the new current key and
all previous keys, then re-encrypts each store with the current key through the
atomic write path. If the process crashes before, during, or after any store
replacement, the keyring can decrypt both rotated and not-yet-rotated stores.
Re-run rotation (which may introduce another current key) or restore the backup.
Do not prune until all encrypted stores have been supplied and verified; prune
checks that every named existing store authenticates with the current key before
removing previous keys. A crash after a store rename but before directory sync
can leave durability uncertain, so retain the pre-rotation backup until the new
backup has been tested.

The storage package cannot discover encrypted stores created by downstream
code. Operators adding such a store must include its name in both calls. Never
rotate or prune while the service is running; another process could write with
a stale key after verification.
