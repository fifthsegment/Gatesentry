<script lang="ts">
  import { _ } from "svelte-i18n";
  import {
    Laptop,
    Certificate,
    Download,
    Globe,
    Settings,
    ChevronDown,
    Copy,
    Checkmark,
  } from "carbon-icons-svelte";
  import { getBasePath } from "../../lib/navigate";

  export let proxyHost = "";
  export let proxyPort = "10413";
  export let adminPort = "";

  let downloadLink = getBasePath() + "/api/files/certificate";
  let openSection: string | null = null;

  function toggle(id: string) {
    openSection = openSection === id ? null : id;
  }

  $: displayHost = proxyHost || "your-gatesentry-ip";

  function buildPacUrl(host: string, port: string): string {
    const h = host?.trim() || window.location.hostname;
    const p = port || "";
    if (p && p !== "80" && p !== "443") {
      return `http://${h}:${p}/wpad.dat`;
    }
    return `http://${h}/wpad.dat`;
  }

  $: pacUrl = buildPacUrl(proxyHost, adminPort);

  let copied = false;
  function copyUrl() {
    navigator.clipboard.writeText(pacUrl);
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }
</script>

<!-- ── Step 1: Configure Proxy ── -->
<div class="setup-step">
  <div class="step-header">
    <div class="step-num">1</div>
    <Globe size={20} />
    <h5>{$_("Configure your browser or OS to use the proxy")}</h5>
  </div>

  <p class="step-desc">
    {$_(
      "Choose the method that works best for your setup. The recommended approach uses a PAC URL which automatically routes traffic through GateSentry.",
    )}
  </p>

  <!-- Option A: PAC URL (recommended) -->
  <div class="method-card method-card--recommended">
    <div class="method-badge">{$_("Recommended")}</div>
    <h5>
      <Settings size={16} style="position:relative;top:2px;margin-right:4px;" />
      {$_("Automatic — PAC URL")}
    </h5>
    <p>
      {$_(
        "Set your system or browser to use this automatic configuration script:",
      )}
    </p>
    <div class="pac-url">
      <code>{pacUrl}</code>
      <button
        class="copy-btn"
        on:click|stopPropagation={copyUrl}
        title={$_("Copy to clipboard")}
      >
        {#if copied}
          <Checkmark size={16} />
        {:else}
          <Copy size={16} />
        {/if}
      </button>
    </div>
    <p class="step-hint">
      {$_(
        "Configure this in your OS proxy settings under 'Automatic proxy configuration' or 'Use setup script'.",
      )}
    </p>

    <button class="expand-btn" on:click={() => toggle("pac")}>
      {$_("Show OS-specific steps")}
      <ChevronDown
        size={16}
        style="transition:transform .2s;transform:rotate({openSection === 'pac'
          ? 180
          : 0}deg)"
      />
    </button>
    {#if openSection === "pac"}
      <div class="expand-body">
        <div class="os-card">
          <h6>Windows 10 / 11</h6>
          <ol>
            <li>
              {$_("Open")}
              <strong>Settings → Network & Internet → Proxy</strong>
            </li>
            <li>
              {$_("Under 'Automatic proxy setup', enable")}
              <strong>{$_("Use setup script")}</strong>
            </li>
            <li>
              {$_("Enter the PAC URL shown above and click")}
              <strong>{$_("Save")}</strong>
            </li>
          </ol>
        </div>
        <div class="os-card">
          <h6>macOS</h6>
          <ol>
            <li>{$_("Open")} <strong>System Settings → Network</strong></li>
            <li>
              {$_("Select your active connection, click")}
              <strong>{$_("Details → Proxies")}</strong>
            </li>
            <li>
              {$_("Enable")}
              <strong>{$_("Automatic Proxy Configuration")}</strong>
              {$_("and paste the PAC URL")}
            </li>
          </ol>
        </div>
        <div class="os-card">
          <h6>Linux (GNOME / KDE)</h6>
          <ol>
            <li>
              {$_("Open")} <strong>Settings → Network → Network Proxy</strong>
            </li>
            <li>{$_("Set method to")} <strong>{$_("Automatic")}</strong></li>
            <li>{$_("Enter the PAC URL in the Configuration URL field")}</li>
          </ol>
        </div>
        <div class="os-card">
          <h6>Firefox ({$_("any OS")})</h6>
          <ol>
            <li>
              {$_("Open")}
              <strong>Settings → General → Network Settings → Settings</strong>
            </li>
            <li>
              {$_("Select")}
              <strong>{$_("Automatic proxy configuration URL")}</strong>
            </li>
            <li>
              {$_("Enter the PAC URL and click")} <strong>{$_("OK")}</strong>
            </li>
          </ol>
        </div>
      </div>
    {/if}
  </div>

  <!-- Option B: Manual proxy -->
  <div class="method-card">
    <h5>
      <Laptop size={16} style="position:relative;top:2px;margin-right:4px;" />
      {$_("Manual — Enter proxy host & port")}
    </h5>
    <p>
      {$_(
        "If PAC URLs are not available, configure the proxy manually in your browser or OS settings:",
      )}
    </p>
    <div class="proxy-info">
      <div class="proxy-field">
        <span class="proxy-label">{$_("Proxy Host")}</span>
        <code>{displayHost}</code>
      </div>
      <div class="proxy-field">
        <span class="proxy-label">{$_("Proxy Port")}</span>
        <code>{proxyPort}</code>
      </div>
    </div>
    <p class="step-hint">
      {$_(
        "Set this as both the HTTP and HTTPS proxy. Most browsers use the OS proxy settings — Chrome, Edge, and Safari all follow the system configuration.",
      )}
    </p>
  </div>
</div>

<!-- ── Step 2: Install Certificate ── -->
<div class="setup-step">
  <div class="step-header">
    <div class="step-num">2</div>
    <Certificate size={20} />
    <h5>{$_("Install the CA certificate")}</h5>
  </div>

  <p class="step-desc">
    {$_(
      "This step is only required if MITM (HTTPS) filtering is enabled. The certificate allows GateSentry to inspect encrypted traffic for content filtering.",
    )}
  </p>

  <a href={downloadLink} target="_blank" class="download-card">
    <Download size={20} />
    <span>{$_("Download CA Certificate")}</span>
  </a>

  <button class="expand-btn" on:click={() => toggle("cert")}>
    {$_("Show installation steps per OS")}
    <ChevronDown
      size={16}
      style="transition:transform .2s;transform:rotate({openSection === 'cert'
        ? 180
        : 0}deg)"
    />
  </button>
  {#if openSection === "cert"}
    <div class="expand-body">
      <div class="os-card">
        <h6>Windows</h6>
        <ol>
          <li>
            {$_("Double-click the downloaded")} <code>gatesentry-ca.crt</code>
            {$_("file")}
          </li>
          <li>
            {$_("Click")} <strong>{$_("Install Certificate")}</strong> →
            <strong>{$_("Local Machine")}</strong>
          </li>
          <li>
            {$_("Select")}
            <strong
              >{$_("Place all certificates in the following store")}</strong
            >
            → <strong>{$_("Trusted Root Certification Authorities")}</strong>
          </li>
          <li>{$_("Click")} <strong>{$_("Finish")}</strong></li>
        </ol>
      </div>
      <div class="os-card">
        <h6>macOS</h6>
        <ol>
          <li>
            {$_("Double-click the")} <code>.crt</code>
            {$_("file to open Keychain Access")}
          </li>
          <li>
            {$_("Select the")} <strong>{$_("System")}</strong>
            {$_("keychain")}
          </li>
          <li>
            {$_("Find the certificate, double-click it, expand")}
            <strong>{$_("Trust")}</strong>
          </li>
          <li>
            {$_("Set 'When using this certificate' to")}
            <strong>{$_("Always Trust")}</strong>
          </li>
        </ol>
      </div>
      <div class="os-card">
        <h6>Linux</h6>
        <ol>
          <li>
            {$_("Copy the")} <code>.crt</code>
            {$_("file to")} <code>/usr/local/share/ca-certificates/</code>
          </li>
          <li>{$_("Run:")} <code>sudo update-ca-certificates</code></li>
          <li>
            {$_(
              "For Firefox/Chrome: import via browser Settings → Privacy & Security → Certificates → Import",
            )}
          </li>
        </ol>
      </div>
    </div>
  {/if}
</div>

<style>
  .setup-step {
    margin-bottom: 1.5rem;
  }
  .step-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  .step-header h5 {
    margin: 0;
    font-weight: 600;
  }
  .step-num {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: #0f62fe;
    color: #fff;
    font-size: 0.8125rem;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .step-desc {
    font-size: 0.875rem;
    color: #525252;
    line-height: 1.5;
    margin-bottom: 1rem;
  }
  .step-hint {
    font-size: 0.8125rem;
    color: #6f6f6f;
    margin-top: 0.5rem;
    line-height: 1.4;
  }

  .method-card {
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    background: #fff;
    padding: 16px;
    margin-bottom: 12px;
    position: relative;
  }
  .method-card--recommended {
    border-color: #0f62fe;
    background: #f0f4ff;
  }
  .method-badge {
    position: absolute;
    top: -10px;
    right: 12px;
    background: #0f62fe;
    color: #fff;
    font-size: 0.6875rem;
    font-weight: 600;
    padding: 2px 10px;
    border-radius: 10px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .method-card h5 {
    margin: 0 0 0.5rem 0;
    font-size: 0.9375rem;
    font-weight: 600;
  }
  .method-card p {
    font-size: 0.875rem;
    color: #393939;
    line-height: 1.5;
    margin: 0 0 0.5rem 0;
  }

  .pac-url {
    display: flex;
    align-items: center;
    background: #161616;
    border-radius: 4px;
    margin: 0.5rem 0;
  }
  .pac-url code {
    flex: 1;
    color: #78a9ff;
    padding: 10px 14px;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.875rem;
    word-break: break-all;
    user-select: all;
  }
  .copy-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    border-left: 1px solid #393939;
    padding: 0 12px;
    cursor: pointer;
    color: #a8a8a8;
    transition: color 0.15s;
  }
  .copy-btn:hover {
    color: #fff;
  }

  .proxy-info {
    display: flex;
    gap: 1rem;
    margin: 0.5rem 0;
    flex-wrap: wrap;
  }
  .proxy-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .proxy-label {
    font-size: 0.75rem;
    color: #6f6f6f;
    text-transform: uppercase;
    letter-spacing: 0.32px;
  }
  .proxy-field code {
    background: #161616;
    color: #78a9ff;
    padding: 6px 12px;
    border-radius: 4px;
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.875rem;
  }

  .download-card {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    background: #0f62fe;
    color: #fff;
    padding: 10px 20px;
    border-radius: 4px;
    text-decoration: none;
    font-size: 0.875rem;
    font-weight: 500;
    transition: background 0.15s;
    margin-bottom: 1rem;
  }
  .download-card:hover {
    background: #0043ce;
  }

  .expand-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: none;
    color: #0f62fe;
    font-size: 0.8125rem;
    font-weight: 500;
    cursor: pointer;
    padding: 6px 0;
    margin-top: 0.25rem;
  }
  .expand-btn:hover {
    text-decoration: underline;
  }

  .expand-body {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 10px;
    margin-top: 0.75rem;
  }

  .os-card {
    border: 1px solid #e0e0e0;
    border-radius: 4px;
    background: #f4f4f4;
    padding: 12px 14px;
  }
  .os-card h6 {
    margin: 0 0 0.5rem 0;
    font-weight: 600;
    font-size: 0.875rem;
  }
  .os-card ol {
    margin: 0;
    padding-left: 1.25rem;
  }
  .os-card li {
    font-size: 0.8125rem;
    line-height: 1.5;
    margin-bottom: 0.375rem;
    color: #393939;
  }
  .os-card code {
    font-size: 0.8125rem;
    background: #e0e0e0;
    padding: 1px 5px;
    border-radius: 3px;
  }

  @media (max-width: 671px) {
    .expand-body {
      grid-template-columns: 1fr;
    }
    .proxy-info {
      flex-direction: column;
    }
  }
</style>
