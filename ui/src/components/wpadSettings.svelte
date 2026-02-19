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
    DataTable,
    ComposedModal,
    ModalHeader,
    ModalBody,
    ModalFooter,
    Toolbar,
    ToolbarContent,
    InlineLoading,
  } from "carbon-components-svelte";
  import { Save, WarningAlt, AddAlt, RowDelete } from "carbon-icons-svelte";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationSuccess,
    createNotificationError,
  } from "../lib/utils";
  import { getBasePath } from "../lib/navigate";

  let proxyHost = "";
  let proxyPort = "10413";
  let configured = false;
  let loading = true;
  let saving = false;
  let pacPreview = "";

  // Clipboard fallback for non-HTTPS contexts (navigator.clipboard
  // requires a secure context). Falls back to the legacy
  // document.execCommand("copy") approach.
  function copyToClipboard(text: string) {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).catch(() => fallbackCopy(text));
    } else {
      fallbackCopy(text);
    }
  }

  function fallbackCopy(text: string) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    document.body.removeChild(ta);
  }

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

  // ── Proxy Bypass Lists ──
  let allLists: any[] = [];
  let bypassListIds: string[] = [];
  let bypassLoaded = false;
  let showBypassPicker = false;

  function getHeaders() {
    const token = localStorage.getItem("jwt");
    return {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };
  }

  async function loadAllDomainLists() {
    try {
      const res = await fetch(getBasePath() + "/api/domainlists", {
        headers: getHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        allLists = data.lists || [];
      }
    } catch (e) {
      console.error("Failed to load domain lists:", e);
    }
  }

  async function loadBypassListIds() {
    try {
      const resp = await $store.api.getSetting("wpad_bypass_domain_lists");
      if (resp && resp.Value) {
        bypassListIds = JSON.parse(resp.Value);
      } else {
        bypassListIds = [];
      }
    } catch {
      bypassListIds = [];
    }
  }

  async function saveBypassListIds() {
    try {
      await $store.api.setSetting(
        "wpad_bypass_domain_lists",
        JSON.stringify(bypassListIds),
      );
      notificationstore.add(
        createNotificationSuccess(
          {
            title: $_("Success"),
            subtitle: $_(
              "Proxy bypass list updated. PAC file will reflect changes on next client refresh.",
            ),
          },
          $_,
        ),
      );
      // Refresh PAC preview to show updated bypass domains
      await refreshPreview();
    } catch {
      notificationstore.add(
        createNotificationError(
          { title: $_("Error"), subtitle: $_("Unable to save bypass list") },
          $_,
        ),
      );
    }
  }

  function findList(id: string) {
    return allLists.find((l: any) => l.id === id);
  }

  function buildBypassRows(ids: string[]) {
    return ids.map((id) => {
      const list = findList(id);
      return {
        id: id,
        name: list ? list.name : id,
        source: list ? list.source : "—",
        category: list ? list.category || "—" : "—",
        entry_count: list ? list.entry_count || 0 : 0,
        actions: "",
      };
    });
  }

  function getAvailableForBypass() {
    return allLists.filter((l: any) => !bypassListIds.includes(l.id));
  }

  async function addBypassList(listId: string) {
    bypassListIds = [...bypassListIds, listId];
    await saveBypassListIds();
    showBypassPicker = false;
  }

  async function removeBypassList(id: string) {
    bypassListIds = bypassListIds.filter((i) => i !== id);
    await saveBypassListIds();
  }

  // Load bypass lists alongside other settings
  async function loadBypassData() {
    await Promise.all([loadAllDomainLists(), loadBypassListIds()]);
    bypassLoaded = true;
  }

  // Trigger bypass data load on mount alongside settings
  onMount(() => {
    loadSettings();
    loadBypassData();
  });
</script>

<div class="wpad-section">
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
            <WarningAlt size={16} style="margin-right:4px;" />
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
            <CodeSnippet type="single" code={pacUrl} copy={copyToClipboard} />
          </div>
        </div>

        {#if pacPreview}
          <div class="wpad-pac-preview">
            <span class="wpad-info-label">{$_("PAC File Preview")}</span>
            <CodeSnippet
              type="multi"
              code={pacPreview}
              copy={copyToClipboard}
            />
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

    <!-- ── Proxy Bypass Lists ── -->
    <Tile class="wpad-bypass-tile">
      <div class="wpad-config-header">
        <strong>{$_("Proxy Bypass Lists")}</strong>
        {#if bypassListIds.length > 0}
          <Tag type="cyan" size="sm">
            {bypassListIds.length}
            {bypassListIds.length === 1 ? $_("list") : $_("lists")}
          </Tag>
        {/if}
      </div>
      <p class="wpad-config-desc">
        {$_(
          "Domains in these lists will bypass the proxy entirely (connect DIRECT). Use this for applications that don't support proxy authentication, such as 1Password, GitHub Copilot, or apps with certificate pinning. The PAC file served to clients will include these domains as bypass entries.",
        )}
      </p>

      {#if !bypassLoaded}
        <InlineLoading description="Loading bypass lists..." />
      {:else}
        <DataTable
          size="medium"
          headers={[
            { key: "name", value: $_("Name") },
            { key: "source", value: $_("Source") },
            { key: "category", value: $_("Category") },
            { key: "entry_count", value: $_("Domains") },
            { key: "actions", value: "" },
          ]}
          rows={buildBypassRows(bypassListIds)}
        >
          <Toolbar size="sm">
            <ToolbarContent>
              <Button
                size="small"
                kind="primary"
                icon={AddAlt}
                on:click={() => {
                  showBypassPicker = true;
                }}
              >
                {$_("Add Bypass List")}
              </Button>
            </ToolbarContent>
          </Toolbar>
          <svelte:fragment slot="cell" let:row let:cell>
            {#if cell.key === "actions"}
              <div style="float: right;">
                <Button
                  size="small"
                  kind="danger-ghost"
                  icon={RowDelete}
                  iconDescription={$_("Remove")}
                  on:click={() => removeBypassList(row.id)}
                />
              </div>
            {:else if cell.key === "source"}
              <Tag size="sm" type={cell.value === "url" ? "blue" : "green"}>
                {cell.value === "url" ? "URL" : "Local"}
              </Tag>
            {:else if cell.key === "entry_count"}
              <strong>{cell.value.toLocaleString()}</strong>
            {:else}
              {cell.value}
            {/if}
          </svelte:fragment>
        </DataTable>

        {#if bypassListIds.length === 0}
          <p class="wpad-bypass-empty">
            {$_(
              "No bypass lists configured. All traffic will be routed through the proxy as defined by the PAC file above.",
            )}
          </p>
        {/if}
      {/if}
    </Tile>

    {#if showBypassPicker}
      <ComposedModal
        open
        on:close={() => {
          showBypassPicker = false;
        }}
      >
        <ModalHeader title={$_("Add Domain List to Proxy Bypass")} />
        <ModalBody>
          <p style="margin-bottom: 1rem; color: #525252; font-size: 0.85rem;">
            {$_(
              "Select a domain list. All domains in the list will bypass the proxy (DIRECT connection) — useful for apps that don't support proxy authentication or use certificate pinning.",
            )}
          </p>
          {#if getAvailableForBypass().length === 0}
            <p style="padding: 16px 0; color: #525252;">
              {$_(
                "No domain lists available. Create one on the Domain Lists page first.",
              )}
            </p>
          {:else}
            <DataTable
              size="compact"
              headers={[
                { key: "name", value: $_("Name") },
                { key: "source", value: $_("Source") },
                { key: "category", value: $_("Category") },
                { key: "entry_count", value: $_("Domains") },
                { key: "pick", value: "" },
              ]}
              rows={getAvailableForBypass().map((l) => ({
                id: l.id,
                name: l.name,
                source: l.source,
                category: l.category || "—",
                entry_count: l.entry_count || 0,
                pick: "",
              }))}
            >
              <svelte:fragment slot="cell" let:row let:cell>
                {#if cell.key === "pick"}
                  <Button
                    size="small"
                    kind="primary"
                    icon={AddAlt}
                    iconDescription={$_("Add")}
                    on:click={() => addBypassList(row.id)}
                  />
                {:else if cell.key === "source"}
                  <Tag size="sm" type={cell.value === "url" ? "blue" : "green"}>
                    {cell.value === "url" ? "URL" : "Local"}
                  </Tag>
                {:else if cell.key === "entry_count"}
                  <strong>{cell.value.toLocaleString()}</strong>
                {:else}
                  {cell.value}
                {/if}
              </svelte:fragment>
            </DataTable>
          {/if}
        </ModalBody>
        <ModalFooter>
          <Button kind="secondary" on:click={() => (showBypassPicker = false)}>
            {$_("Cancel")}
          </Button>
        </ModalFooter>
      </ComposedModal>
    {/if}
  {/if}
</div>

<style>
  .wpad-section {
    margin-top: 0;
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
  :global(.wpad-bypass-tile) {
    margin-top: 1rem;
    margin-bottom: 1rem;
  }
  .wpad-bypass-empty {
    padding: 1rem 0 0.5rem;
    color: #525252;
    font-size: 0.85rem;
    font-style: italic;
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

  @media (max-width: 671px) {
    .wpad-input-row {
      flex-direction: column;
      align-items: stretch;
    }
    .wpad-input-port {
      max-width: none;
    }
    .wpad-input-save {
      padding-bottom: 0;
    }
  }
</style>
