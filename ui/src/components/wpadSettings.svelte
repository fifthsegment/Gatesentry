<script lang="ts">
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { onMount } from "svelte";
  import Toggle from "./toggle.svelte";
  import {
    Tile,
    InlineNotification,
    CodeSnippet,
    TextInput,
    Button,
    Tag,
  } from "carbon-components-svelte";
  import { Network_3, Save, WarningAlt } from "carbon-icons-svelte";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationSuccess,
    createNotificationError,
  } from "../lib/utils";

  let proxyHost = "";
  let proxyPort = "10413";
  let configured = false;
  let loading = true;
  let saving = false;
  let pacPreview = "";

  // The admin port comes from the backend — it knows what port it's
  // listening on.  We never use window.location.port because that may
  // be the Vite dev-server or a reverse-proxy frontend.
  let adminPort = "";

  // PAC URL is derived from the best-known host + backend admin port.
  // When the admin has configured wpad_proxy_host we use that; otherwise
  // we fall back to window.location.hostname (the URL the admin is
  // currently using to reach the UI — a reasonable guess).
  // The port MUST come from the backend (adminPort), never from
  // window.location.port (which is the UI port, not the backend port).
  $: pacUrl = buildPacUrl(proxyHost, adminPort);

  function buildPacUrl(host: string, port: string): string {
    const h = host?.trim() || window.location.hostname;
    const p = port || "";
    if (p && p !== "80" && p !== "443") {
      return `http://${h}:${p}/wpad.dat`;
    }
    return `http://${h}/wpad.dat`;
  }

  async function loadSettings() {
    try {
      const [hostResp, portResp, wpadInfo] = await Promise.all([
        $store.api.getSetting("wpad_proxy_host"),
        $store.api.getSetting("wpad_proxy_port"),
        $store.api.doCall("/wpad/info").catch(() => null),
      ]);
      proxyHost = hostResp?.Value ?? "";
      proxyPort = portResp?.Value || "10413";
      configured = proxyHost !== "";
      adminPort = wpadInfo?.adminPort ?? "";
      pacPreview = wpadInfo?.pacFile ?? "";
    } catch (e) {
      console.error("Failed to load WPAD settings:", e);
    } finally {
      loading = false;
    }
  }

  async function refreshPreview() {
    try {
      const resp = await $store.api.doCall("/wpad/info");
      adminPort = resp?.adminPort ?? adminPort;
      pacPreview = resp?.pacFile ?? "";
    } catch (e) {
      pacPreview = "";
    }
  }

  async function saveSettings() {
    saving = true;
    try {
      const hostOk = await $store.api.setSetting(
        "wpad_proxy_host",
        proxyHost.trim(),
      );
      const portOk = await $store.api.setSetting(
        "wpad_proxy_port",
        proxyPort.trim() || "10413",
      );

      if (hostOk !== false && portOk !== false) {
        configured = proxyHost.trim() !== "";
        notificationstore.add(
          createNotificationSuccess(
            { subtitle: $_("Proxy address saved") },
            $_,
          ),
        );
        await refreshPreview();
      } else {
        notificationstore.add(
          createNotificationError(
            { subtitle: $_("Failed to save settings") },
            $_,
          ),
        );
      }
    } catch (e) {
      notificationstore.add(
        createNotificationError(
          { subtitle: $_("Failed to save settings") },
          $_,
        ),
      );
    } finally {
      saving = false;
    }
  }

  onMount(loadSettings);
</script>

<br />
<div class="wpad-section">
  <div class="wpad-header">
    <Network_3 size={20} style="position:relative; top:4px;" />
    <label class="bx--label wpad-title"
      >{$_("Automatic Proxy Discovery (WPAD)")}</label
    >
  </div>

  <Toggle
    settingName="wpad_enabled"
    label={$_("WPAD DNS Auto-Discovery")}
    labelA={$_("Disabled")}
    labelB={$_("Enabled")}
    noNotification={false}
  />

  {#if !loading}
    <Tile class="wpad-config-tile">
      <div class="wpad-config-header">
        <strong>{$_("Proxy Address")}</strong>
        {#if !configured}
          <Tag type="red" size="sm">
            <WarningAlt size={12} style="margin-right:4px;" />
            {$_("Not Configured")}
          </Tag>
        {:else}
          <Tag type="green" size="sm">{$_("Configured")}</Tag>
        {/if}
      </div>
      <p class="wpad-config-desc">
        {$_(
          "Enter the IP address or hostname that devices on your network should use to reach this GateSentry proxy. This is the address that will appear in the PAC file.",
        )}
      </p>
      <div class="wpad-input-row">
        <div class="wpad-input-host">
          <TextInput
            labelText={$_("Proxy Host / IP")}
            placeholder="192.168.1.100"
            bind:value={proxyHost}
            helperText={$_(
              "The LAN IP or hostname clients will use to reach the proxy",
            )}
          />
        </div>
        <div class="wpad-input-port">
          <TextInput
            labelText={$_("Proxy Port")}
            placeholder="10413"
            bind:value={proxyPort}
            helperText={$_("Default: 10413")}
          />
        </div>
        <div class="wpad-input-save">
          <Button
            size="field"
            icon={Save}
            on:click={saveSettings}
            disabled={saving}
          >
            {saving ? $_("Saving...") : $_("Save")}
          </Button>
        </div>
      </div>
    </Tile>

    {#if configured}
      <Tile class="wpad-info-tile">
        <div class="wpad-info-grid">
          <div class="wpad-info-item full-width">
            <span class="wpad-info-label"
              >{$_("PAC File URL (give this to clients)")}</span
            >
            <CodeSnippet type="single" code={pacUrl} />
          </div>
        </div>

        {#if pacPreview}
          <div class="wpad-pac-preview">
            <span class="wpad-info-label">{$_("PAC File Preview")}</span>
            <CodeSnippet type="multi" code={pacPreview} />
          </div>
        {/if}
      </Tile>
    {/if}

    <InlineNotification
      kind="info"
      lowContrast
      title={$_("Setup Guide")}
      subtitle=""
      hideCloseButton
    >
      <div slot="subtitle" class="wpad-instructions">
        <strong>{$_("Option A: Automatic (WPAD DNS)")}</strong>
        <ol>
          <li>
            {$_("Set your router's DHCP DNS to GateSentry's IP address")}
          </li>
          <li>
            {$_(
              "Enable WPAD DNS Auto-Discovery toggle above — GateSentry will answer wpad.* DNS queries with its own IP",
            )}
          </li>
          <li>
            {$_(
              'Ensure "Automatically detect settings" is enabled in Windows proxy settings (it is by default)',
            )}
          </li>
          <li>
            {$_(
              "Note: Requires GateSentry admin to run on port 80, which may not be possible on Docker/NAS setups",
            )}
          </li>
        </ol>
        <br />
        <strong>{$_("Option B: Manual PAC URL (recommended)")}</strong>
        <ol>
          <li>
            {$_("On each device, go to proxy settings")}
          </li>
          <li>
            {$_(
              'Select "Use automatic configuration script" / "Use setup script"',
            )}
          </li>
          <li>
            {$_("Enter the PAC File URL shown above")}
          </li>
          <li>
            {$_("Works on any port — no port 80 requirement")}
          </li>
        </ol>
      </div>
    </InlineNotification>
  {/if}
</div>

<style>
  .wpad-section {
    margin-top: 1rem;
  }
  .wpad-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  .wpad-title {
    font-size: 0.95rem;
    font-weight: 600;
    margin: 0;
  }
  :global(.wpad-config-tile) {
    margin-bottom: 1rem;
  }
  .wpad-config-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .wpad-config-desc {
    font-size: 0.825rem;
    color: #525252;
    margin-bottom: 1rem;
    line-height: 1.4;
  }
  .wpad-input-row {
    display: flex;
    gap: 1rem;
    align-items: flex-end;
  }
  .wpad-input-host {
    flex: 2;
  }
  .wpad-input-port {
    flex: 1;
    max-width: 140px;
  }
  .wpad-input-save {
    padding-bottom: 1.25rem;
  }
  :global(.wpad-info-tile) {
    margin-bottom: 1rem;
  }
  .wpad-info-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 1rem;
  }
  .wpad-info-item.full-width {
    grid-column: 1 / -1;
  }
  .wpad-info-label {
    display: block;
    font-size: 0.75rem;
    color: #525252;
    margin-bottom: 0.25rem;
  }
  .wpad-pac-preview {
    margin-top: 1rem;
  }
  .wpad-instructions {
    font-size: 0.85rem;
    line-height: 1.5;
  }
  .wpad-instructions ol {
    padding-left: 1.25rem;
    margin-top: 0.5rem;
  }
  .wpad-instructions li {
    margin-bottom: 0.5rem;
  }
</style>
