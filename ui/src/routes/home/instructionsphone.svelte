<script lang="ts">
  import { _ } from "svelte-i18n";
  import {
    Mobile,
    Certificate,
    Download,
    Globe,
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
    <h5>{$_("Configure your phone to use the proxy")}</h5>
  </div>

  <p class="step-desc">
    {$_(
      "Choose the method that works best for your device. The recommended approach uses a PAC URL for automatic proxy configuration.",
    )}
  </p>

  <!-- Option A: PAC URL (recommended) -->
  <div class="method-card method-card--recommended">
    <div class="method-badge">{$_("Recommended")}</div>
    <h5>
      <Mobile size={16} style="position:relative;top:2px;margin-right:4px;" />
      {$_("Automatic — PAC URL")}
    </h5>
    <p>
      {$_(
        "Configure your device to use this automatic proxy configuration URL:",
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

    <button class="expand-btn" on:click={() => toggle("pac")}>
      {$_("Show device-specific steps")}
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
          <h6>iOS (iPhone / iPad)</h6>
          <ol>
            <li>{$_("Open")} <strong>Settings → Wi-Fi</strong></li>
            <li>
              {$_("Tap the")} <strong>ⓘ</strong>
              {$_("icon next to your connected network")}
            </li>
            <li>
              {$_("Scroll down and tap")}
              <strong>{$_("Configure Proxy")}</strong>
            </li>
            <li>
              {$_("Select")} <strong>{$_("Automatic")}</strong>
              {$_("and enter the PAC URL above")}
            </li>
            <li>{$_("Tap")} <strong>{$_("Save")}</strong></li>
          </ol>
        </div>
        <div class="os-card">
          <h6>Android</h6>
          <ol>
            <li>
              {$_("Open")}
              <strong>Settings → Network & Internet → Wi-Fi</strong>
            </li>
            <li>
              {$_("Long-press your connected network and tap")}
              <strong>{$_("Modify network")}</strong>
            </li>
            <li>{$_("Tap")} <strong>{$_("Advanced options")}</strong></li>
            <li>
              {$_("Set Proxy to")} <strong>{$_("Proxy Auto-Config")}</strong>
            </li>
            <li>
              {$_("Enter the PAC URL above and tap")}
              <strong>{$_("Save")}</strong>
            </li>
          </ol>
        </div>
      </div>
    {/if}
  </div>

  <!-- Option B: Manual proxy -->
  <div class="method-card">
    <h5>
      <Mobile size={16} style="position:relative;top:2px;margin-right:4px;" />
      {$_("Manual — Enter proxy host & port")}
    </h5>
    <p>
      {$_(
        "If PAC URLs are not supported, configure the proxy manually in your Wi-Fi settings:",
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

    <button class="expand-btn" on:click={() => toggle("manual")}>
      {$_("Show device-specific steps")}
      <ChevronDown
        size={16}
        style="transition:transform .2s;transform:rotate({openSection ===
        'manual'
          ? 180
          : 0}deg)"
      />
    </button>
    {#if openSection === "manual"}
      <div class="expand-body">
        <div class="os-card">
          <h6>iOS (iPhone / iPad)</h6>
          <ol>
            <li>{$_("Open")} <strong>Settings → Wi-Fi</strong></li>
            <li>
              {$_("Tap the")} <strong>ⓘ</strong>
              {$_("icon next to your connected network")}
            </li>
            <li>
              {$_("Scroll down and tap")}
              <strong>{$_("Configure Proxy")}</strong>
            </li>
            <li>{$_("Select")} <strong>{$_("Manual")}</strong></li>
            <li>{$_("Enter the proxy host and port shown above")}</li>
            <li>{$_("Tap")} <strong>{$_("Save")}</strong></li>
          </ol>
        </div>
        <div class="os-card">
          <h6>Android</h6>
          <ol>
            <li>
              {$_("Open")}
              <strong>Settings → Network & Internet → Wi-Fi</strong>
            </li>
            <li>
              {$_("Long-press your connected network and tap")}
              <strong>{$_("Modify network")}</strong>
            </li>
            <li>{$_("Tap")} <strong>{$_("Advanced options")}</strong></li>
            <li>{$_("Set Proxy to")} <strong>{$_("Manual")}</strong></li>
            <li>{$_("Enter the host and port shown above")}</li>
            <li>{$_("Tap")} <strong>{$_("Save")}</strong></li>
          </ol>
        </div>
      </div>
    {/if}
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
      "This step is only required if MITM (HTTPS) filtering is enabled. Download the certificate on your device, then follow the steps for your platform.",
    )}
  </p>

  <a href={downloadLink} target="_blank" class="download-card">
    <Download size={20} />
    <span>{$_("Download CA Certificate")}</span>
  </a>

  <p class="step-hint">
    {$_(
      "Open this link in your phone's browser while connected through the proxy, or transfer the file via email or AirDrop.",
    )}
  </p>

  <button class="expand-btn" on:click={() => toggle("cert")}>
    {$_("Show installation steps per device")}
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
        <h6>iOS (iPhone / iPad)</h6>
        <ol>
          <li>
            {$_(
              "Open the downloaded certificate file — a prompt will appear to install the profile",
            )}
          </li>
          <li>
            {$_("Go to")}
            <strong>Settings → General → VPN & Device Management</strong>
          </li>
          <li>
            {$_("Tap the GateSentry profile and tap")}
            <strong>{$_("Install")}</strong>
          </li>
          <li>
            {$_("Then go to")}
            <strong
              >Settings → General → About → Certificate Trust Settings</strong
            >
          </li>
          <li>{$_("Enable full trust for the GateSentry root certificate")}</li>
        </ol>
      </div>
      <div class="os-card">
        <h6>Android</h6>
        <ol>
          <li>{$_("Open the downloaded certificate file")}</li>
          <li>
            {$_("Or go to")}
            <strong
              >Settings → Security → Encryption & credentials → Install from
              storage</strong
            >
          </li>
          <li>{$_("Locate and tap the certificate file")}</li>
          <li>
            {$_("Name the certificate (e.g. 'GateSentry') and select")}
            <strong>{$_("VPN and apps")}</strong>
            {$_("or")} <strong>{$_("Wi-Fi")}</strong>
          </li>
          <li>
            {$_("Tap")} <strong>{$_("OK")}</strong>
            {$_("to complete installation")}
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
    margin-top: 0.25rem;
    margin-bottom: 0.75rem;
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
    margin-bottom: 0.5rem;
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
  @media (max-width: 671px) {
    .expand-body {
      grid-template-columns: 1fr;
    }
    .proxy-info {
      flex-direction: column;
    }
  }
</style>
