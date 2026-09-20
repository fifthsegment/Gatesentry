# Secure first-run administration

Fresh installations do not have a default administrator password. Open the dashboard and create the administrator on the setup screen. Setup is not authenticated: the first client that submits the form owns the installation, whether it connects from the gateway host or from any other machine that can reach the dashboard. Complete setup before exposing the admin port, or restrict that port to a trusted management network. GateSentry logs a warning at startup while setup is incomplete.

GateSentry stores the administrator password as a bcrypt hash in the encrypted `GSSettings` MapStore. Setup completion, the username, random JWT signing key, and session generation share one atomic auth record. Setup can complete once. Changing the administrator username or password increments the generation and immediately rejects older JWTs. The browser stores the JWT until logout, but never stores the administrator password.

## Docker and unattended setup

To skip the setup screen, create a root-owned file outside the persistent GateSentry data directory with mode `0600` containing the administrator credentials:

- `{"username":"...","password":"..."}` completes setup during startup.

Mount the file read-only and set `GATESENTRY_BOOTSTRAP_FILE` to its container path. Never put these values in Compose environment variables, command-line arguments, URLs, or logs. The file must be a regular file with no group or other permissions. Unknown fields, incomplete credentials, passwords outside 12 to 72 bytes, and unreadable files stop startup.

After setup succeeds, remove the mount and configuration. Restarting with a completed auth record does not read the file and cannot reopen setup.

Earlier releases also accepted `{"authorization":"..."}`, a one-time value that let an operator complete setup from another machine without first reaching the dashboard. Setup no longer asks for that value, and a file in the old shape stops startup with `bootstrap secret file is malformed or contains unsupported fields`. Remove the file and the `GATESENTRY_BOOTSTRAP_FILE` setting, or replace its contents with the `username`/`password` form.

## Upgrade, rollout, and recovery

On first upgraded startup, existing `general_settings` administrator credentials, including `admin/admin`, are accepted as a compatibility migration. GateSentry hashes the password and removes both legacy credential fields in the same atomic storage update. A read, parse, hash, or write failure aborts startup without changing the previous data, preventing a silent lockout. The general settings API no longer returns or accepts durable plaintext credentials.

Back up the GateSentry data directory before upgrading. Rolling back after migration is constrained: older releases only understand plaintext credentials and cannot authenticate against the new auth record. Restore the pre-upgrade backup to roll back. There is no password recovery or reset bypass; if credentials and all active sessions are lost, restore a known backup or deliberately reinitialize the settings store and repeat setup.

Bind the admin service to a trusted management network or put it behind a correctly secured reverse proxy, and complete setup before that port is reachable from an untrusted network. First-run setup is not authenticated and does not trust proxy headers, so any client that can load the setup page can claim the administrator account until setup succeeds. Reload the setup page after upgrading: an older cached bundle posts an `authorization` field that the current endpoint rejects with `400`. HTTPS inspection remains advanced and opt-in; first-run setup does not change DNS/proxy filtering behavior or imply VPN, QUIC, malware, URL, MIME, or content coverage.
