# Secure first-run administration

Fresh installations do not have a default administrator password. Open the dashboard from the GateSentry host using `http://127.0.0.1:10786` or `http://[::1]:10786` and create the administrator on the setup screen. Loopback is determined from the TCP peer address. Requests containing `Forwarded`, `X-Forwarded-For`, or `X-Real-IP` are treated as remote because GateSentry has no trusted-proxy configuration. To set up from another machine or through a reverse proxy, configure a one-time authorization file as described below.

GateSentry stores the administrator password as a bcrypt hash in the encrypted `GSSettings` MapStore. Setup completion, the username, authorization digest, random JWT signing key, and session generation share one atomic auth record. Setup can complete once. Changing the administrator username or password increments the generation and immediately rejects older JWTs. The browser stores the JWT until logout, but never stores the administrator password.

## Docker and unattended setup

Create a root-owned file outside the persistent GateSentry data directory with mode `0600`. Choose exactly one JSON form:

- `{"authorization":"..."}` permits remote interactive setup with that one-time value. Generate exactly 32 random bytes and encode them as unpadded base64url. For example, `python3 -c 'import secrets; print(secrets.token_urlsafe(32))'` prints a suitable value.
- `{"username":"...","password":"..."}` completes setup during startup.

Mount the file read-only and set `GATESENTRY_BOOTSTRAP_FILE` to its container path. Never put these values in Compose environment variables, command-line arguments, URLs, or logs. The file must be a regular file with no group or other permissions. Unknown fields, mixed modes, incomplete credentials, passwords outside 12 to 72 bytes, authorization values outside the exact format above, and unreadable files stop startup.

After setup succeeds, remove the mount and configuration. Restarting with a completed auth record does not read the file and cannot reopen setup. The authorization value is stored only as a digest and is erased when consumed.

## Upgrade, rollout, and recovery

On first upgraded startup, existing `general_settings` administrator credentials, including `admin/admin`, are accepted as a compatibility migration. GateSentry hashes the password and removes both legacy credential fields in the same atomic storage update. A read, parse, hash, or write failure aborts startup without changing the previous data, preventing a silent lockout. The general settings API no longer returns or accepts durable plaintext credentials.

Back up the GateSentry data directory before upgrading. Rolling back after migration is constrained: older releases only understand plaintext credentials and cannot authenticate against the new auth record. Restore the pre-upgrade backup to roll back. There is no password recovery or reset bypass; if credentials and all active sessions are lost, restore a known backup or deliberately reinitialize the settings store and repeat setup.

Bind the admin service to a trusted management network or put it behind a correctly secured reverse proxy. GateSentry does not trust proxy headers for setup. Remote setup requires the one-time authorization file. HTTPS inspection remains advanced and opt-in; first-run setup does not change DNS/proxy filtering behavior or imply VPN, QUIC, malware, URL, MIME, or content coverage.
