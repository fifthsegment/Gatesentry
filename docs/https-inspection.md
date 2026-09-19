# HTTPS inspection and certificate trust

This document explains how to make optional HTTPS (man-in-the-middle) inspection understandable and honest in GateSentry, without overstating coverage. It covers what inspection does, the consent an operator must accept before enabling it, how to install and remove the gateway CA certificate on each platform, a verification step you can actually perform against the running gateway, the exclusions that bypass inspection, and the limits of what inspection can see.

GateSentry is a self-hosted family and small-office internet safety gateway. HTTPS inspection is disabled by default and is an advanced, opt-in deployment. It requires client trust-store changes and, for transparent interception, routing changes. Do not enable it until you understand the privacy implications and have reviewed [SECURITY.md](../SECURITY.md).

## What HTTPS inspection does

When enabled, the proxy intercepts TLS connections that reach the explicit HTTP/HTTPS proxy or a correctly configured Linux transparent REDIRECT/TPROXY path. For each connection that is not on the exclusion list, the proxy generates a leaf certificate signed by the configured CA and presents it to the client. If the client trusts the CA, it accepts the forged certificate and the proxy can inspect the cleartext content against URL, keyword, MIME, and AI image rules.

Hosts on the Exception Hosts list (the `url/https_dontbump` filter, file `filterfiles/dontbump.json`) are tunnelled directly without inspection. This list exists so apps that detect or reject man-in-the-middle filtering can continue to work.

## Consent

Before enabling inspection, the operator must accept this consent statement in the admin UI:

> I understand that enabling HTTPS inspection allows the gateway to decrypt and inspect the content of encrypted traffic from clients that trust the gateway CA certificate. This access can reveal sensitive and personal data of users on the network. I have a legitimate purpose, I will obtain consent where applicable, I will protect the data directory and every backup containing the CA key, and I will remove the CA from clients when this deployment is retired or compromised. I understand that enabling inspection does not inspect every client and does not provide universal HTTPS, VPN, QUIC, or malware coverage.

The UI presents this consent as a gate before the toggle that enables inspection. Toggling the setting on without accepting the consent is not sufficient to claim coverage.

## Install the gateway CA certificate

Download the CA certificate from the admin UI (Settings, HTTPS filtering, Download Certificate) or from the authenticated endpoint `GET /api/files/certificate`. The file is PEM-encoded.

### iOS / iPadOS

**Install:**
1. Email the `certificate.pem` file to an account on the device, or host it on a web page the device can reach, or transfer it via Files.
2. Open the file on the device. iOS prompts to download a configuration profile.
3. Open Settings, tap "Profile Downloaded" near the top.
4. Tap Install, enter the device passcode, and confirm.
5. Go to Settings, General, About, Certificate Trust Settings.
6. Enable trust for the GateSentry CA under "Enable full trust for root certificates."

**Remove:**
1. Go to Settings, General, VPN & Device Management.
2. Tap the GateSentry profile.
3. Tap Remove Profile and enter the device passcode.

**Verify:** Configure the device to use the GateSentry proxy (Wi-Fi, HTTP Proxy, Manual). Browse to an HTTPS site that is not on the exclusion list. Inspect the certificate chain in the browser (tap the lock icon, View Certificate). The chain must show the GateSentry CA as the issuer of the leaf certificate. If the chain shows the site's real issuer, the certificate is not trusted or the connection bypassed the proxy.

### macOS

**Install:**
1. Download `certificate.pem`.
2. Open Keychain Access.
3. Drag the certificate into the System keychain (not Login).
4. Find the GateSentry certificate, double-click it.
5. Expand Trust, set "When using this certificate" to Always Trust.
6. Close the dialog and enter your administrator password.

**Remove:**
1. Open Keychain Access.
2. Search for the GateSentry certificate in the System keychain.
3. Right-click and Delete, entering your administrator password.

**Verify:** Configure the proxy in System Settings, Network, Wi-Fi, Details, Proxies. Browse to an HTTPS site that is not on the exclusion list. In Safari, click the lock icon, Show Certificate. The chain must show the GateSentry CA as the issuer. If it shows the site's real issuer, trust is not installed or the connection bypassed the proxy.

### Windows

**Install:**
1. Download `certificate.pem`.
2. Right-click the file and choose Install Certificate.
3. Select Local Machine (requires administrator rights).
4. Choose Place all certificates in the following store, Browse, Trusted Root Certification Authorities, OK, Next, Finish.
5. Confirm the security prompt.

**Remove:**
1. Press Win+R, type `certlm.msc`, press Enter.
2. Navigate to Trusted Root Certification Authorities, Certificates.
3. Find the GateSentry certificate, right-click, Delete.

**Verify:** Configure the proxy in Settings, Network & Internet, Proxy. In Edge or Chrome, browse to an HTTPS site that is not on the exclusion list. Click the lock icon, Connection is secure, Certificate is valid. The issuer must be the GateSentry CA. If it shows the site's real issuer, trust is not installed or the connection bypassed the proxy.

### Android

**Install:**
1. Download `certificate.pem` to the device.
2. Open Settings, Security, Encryption & credentials, Install a certificate, CA certificate (the path varies by manufacturer).
3. Select the downloaded file. Confirm the warning.
4. Enter the device PIN or password if prompted.

**Remove:**
1. Open Settings, Security, Encryption & credentials, User credentials or Trusted credentials.
2. Find the GateSentry certificate and remove it.

**Verify:** Configure the Wi-Fi proxy to the GateSentry proxy. In Chrome, browse to an HTTPS site that is not on the exclusion list. Tap the lock icon, Details, Certificate. The issuer must be the GateSentry CA. Android apps that pin certificates may reject the connection regardless of trust-store installation; see the limits section.

### Linux (system trust store plus Firefox)

**Install (system):**
1. Copy `certificate.pem` to `/usr/local/share/ca-certificates/gatesentry.crt` (Debian/Ubuntu) or `/etc/pki/ca-trust/source/anchors/gatesentry.crt` (RHEL/Fedora).
2. Run `sudo update-ca-certificates` (Debian/Ubuntu) or `sudo update-ca-trust` (RHEL/Fedora).

**Remove (system):**
1. Delete the file you copied.
2. Re-run the update command above.

**Install (Firefox):** Firefox uses its own trust store and does not read the system store by default.
1. Open Firefox, Settings, Privacy & Security, scroll to Certificates, View Certificates.
2. Authorities tab, Import.
3. Select `certificate.pem`.
4. Check Trust this CA to identify websites, OK.

**Remove (Firefox):**
1. Firefox, Settings, Privacy & Security, Certificates, View Certificates.
2. Authorities tab, find GateSentry, select it, Delete or Distrust.

**Verify:** Set the `http_proxy` and `https_proxy` environment variables or configure the desktop proxy to the GateSentry proxy. In a browser, browse to an HTTPS site that is not on the exclusion list. Inspect the certificate chain. The issuer must be the GateSentry CA. For Firefox, verify the chain in Firefox specifically since it has a separate trust store.

## Verification step

After installing the CA and configuring the proxy, verify interception against the running gateway:
1. Ensure HTTPS filtering is enabled in Settings and that the target site is not on the Exception Hosts list.
2. Browse to `https://www.example.com` (or any HTTPS site not on the exclusion list).
3. Inspect the certificate chain in the browser. The leaf certificate issuer must be the GateSentry CA.
4. Check the inspection status panel in Settings (HTTPS Inspection Status). The inspect count should increase. If it does not, the connection bypassed the proxy, the client did not trust the CA, or the site is on the exclusion list.
5. Compare against a site on the exclusion list: the certificate chain should show the real issuer, not the GateSentry CA, and the bypass count should increase.

This is a verification you perform, not a claim that interception works. Some clients may reject interception; see the limits below.

## Exclusions

The Exception Hosts list (filter `url/https_dontbump`, filter name "Exception Hosts", file `filterfiles/dontbump.json`) controls which hosts bypass inspection. Hosts on this list are tunnelled directly without TLS inspection. This is not malware protection; it is a list of hosts whose traffic is not inspected, typically because the app detects or rejects MITM.

The admin UI shows this list under HTTPS Inspection Status with explicit wording that listed hosts are tunnelled without inspection. The UI also shows inspect-versus-bypass outcome counts from recorded decisions: inspect corresponds to `ssl-bump` decisions and bypass corresponds to `ssldirect` decisions. These counts describe recorded decisions, not total traffic.

## Limits

HTTPS inspection does not inspect all traffic on a network. The following cases bypass or defeat inspection; GateSentry does not provide blanket HTTPS, VPN, QUIC, or malware coverage.

- **Certificate-pinned clients:** Apps that pin a server certificate or public key reject the forged leaf certificate regardless of CA trust-store installation. This includes many banking, messaging, and media apps.
- **Encrypted DNS (DoH/DoT):** If a client resolves domains via DNS-over-HTTPS or DNS-over-TLS that does not route through the gateway's DNS service, the DNS filtering path is bypassed and the proxy may never see the connection.
- **QUIC/HTTP3 over UDP:** QUIC and HTTP/3 use UDP. The proxy interception paths are TCP-based (explicit proxy CONNECT and Linux transparent REDIRECT/TPROXY). UDP QUIC traffic outside those paths is not inspected.
- **VPN tunnels:** Traffic inside a VPN tunnel is encrypted before it reaches the gateway and cannot be inspected.
- **Unsupported clients:** Clients that do not use the configured proxy or DNS, or that have incompatible trust behavior, are not inspected. Some inspection failures in the current proxy can fall back to a direct connection, which can fail at the client or bypass content inspection.

These limits are verified in this repository's tests and in the existing proxy behavior. For the full security and privacy posture, including supported versions and administrative exposure, see [SECURITY.md](../SECURITY.md) rather than duplicating coverage claims here.
