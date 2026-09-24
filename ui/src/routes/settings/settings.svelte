<script lang="ts">
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
  import ConnectedCertificateComposed from "../../components/connectedCertificateComposed.svelte";
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    InlineLoading,
    InlineNotification,
    Tag,
    TextInput,
  } from "carbon-components-svelte";
  import { store } from "../../store/apistore";

  interface TailscaleStatus {
    enabled: boolean;
    detected: boolean;
    state: string;
    backend_state: string;
    self_name: string;
    last_refresh: string;
    message: string;
  }

  let diagnostics: any = null;
  let bundlePreview = "";
  let loadingDiagnostics = false;
  let diagnosticsError = "";

  let tailscaleStatus: TailscaleStatus | null = null;
  let tailscaleLoading = false;
  let tailscaleSaving = false;
  let tailscaleError = "";
  let tailscaleNotice = "";

  async function loadDiagnostics() {
    loadingDiagnostics = true;
    diagnosticsError = "";
    try {
      diagnostics = await $store.api.doCall("/diagnostics");
    } catch (error) {
      diagnosticsError =
        error instanceof Error ? error.message : "Diagnostics failed";
    } finally {
      loadingDiagnostics = false;
    }
  }

  async function previewBundle() {
    diagnosticsError = "";
    try {
      const bundle = await $store.api.doCall("/diagnostics/bundle");
      bundlePreview = JSON.stringify(bundle, null, 2);
    } catch (error) {
      diagnosticsError =
        error instanceof Error ? error.message : "Bundle preview failed";
    }
  }

  function downloadBundle() {
    if (!bundlePreview) return;
    const url = URL.createObjectURL(
      new Blob([bundlePreview + "\n"], { type: "application/json" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "gatesentry-support-bundle.json";
    link.click();
    URL.revokeObjectURL(url);
  }

  async function loadTailscaleStatus() {
    tailscaleLoading = true;
    tailscaleError = "";
    try {
      tailscaleStatus = await $store.api.doCall("/tailscale/status");
    } catch (error) {
      tailscaleError =
        error instanceof Error ? error.message : "Tailscale status failed";
    } finally {
      tailscaleLoading = false;
    }
  }

  async function setTailscaleEnabled(enabled: boolean) {
    tailscaleSaving = true;
    tailscaleError = "";
    tailscaleNotice = "";
    try {
      await $store.api.doCall("/tailscale/config", "put", { enabled });
      tailscaleNotice = enabled
        ? "Tailscale integration enabled."
        : "Tailscale integration disabled.";
      await loadTailscaleStatus();
    } catch (error) {
      tailscaleError =
        error instanceof Error ? error.message : "Tailscale update failed";
    } finally {
      tailscaleSaving = false;
    }
  }

  function tagType(status: string) {
    return status === "ok" || status === "running" || status === "connected"
      ? "green"
      : status === "failed" || status === "stopped" || status === "unavailable"
      ? "red"
      : status === "degraded" || status === "connecting"
      ? "warm-gray"
      : "gray";
  }

  function tailscaleStatusKind(
    state: string,
  ): "success" | "warning" | "error" | "info" {
    if (state === "connected") return "success";
    if (state === "degraded" || state === "connecting") return "warning";
    if (state === "unavailable" || state === "stopped") return "error";
    return "info";
  }

  function tailscaleStatusTitle(state: string): string {
    if (state === "connected") return "Tailscale connected";
    if (state === "degraded") return "Tailscale data degraded";
    if (state === "unavailable") return "Tailscale unavailable";
    if (state === "connecting") return "Tailscale connecting";
    if (state === "disabled") return "Tailscale disabled";
    return "Tailscale status";
  }

  function formatDate(value: string): string {
    if (!value) return "Never";
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  onMount(() => {
    loadDiagnostics();
    loadTailscaleStatus();
  });
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Settings</BreadcrumbItem>
</Breadcrumb>

<h2>Settings</h2>

<br />

<ConnectedGeneralSettingInput
  keyName="log_location"
  title={$_("Log Location")}
  labelText={$_("Log Location")}
  type="text"
  helperText=""
/>
<br />
<TextInput
  value={$store.api.username}
  type="text"
  title={$_("Admin username")}
  labelText={$_("Admin username")}
  disabled={true}
/>

<HttpsToggle />

<ConnectedCertificateComposed />

<section class="tailscale gs-card">
  <div class="card-heading">
    <div>
      <h3>{$_("Tailscale")}</h3>
      <p>
        {$_(
          "Discover Tailscale peers and explicitly link them to devices in the device inventory.",
        )}
      </p>
    </div>
    {#if tailscaleStatus}
      <Tag type={tailscaleStatus.enabled ? "green" : "gray"} size="sm">
        {tailscaleStatus.enabled ? $_("Enabled") : $_("Disabled")}
      </Tag>
    {/if}
  </div>

  {#if tailscaleError}
    <InlineNotification
      kind="error"
      title="Tailscale error"
      subtitle={tailscaleError}
      on:close={() => (tailscaleError = "")}
    />
  {/if}
  {#if tailscaleNotice}
    <InlineNotification
      kind="success"
      title="Tailscale updated"
      subtitle={tailscaleNotice}
      on:close={() => (tailscaleNotice = "")}
    />
  {/if}

  {#if tailscaleLoading && !tailscaleStatus}
    <InlineLoading description="Checking Tailscale..." />
  {:else if tailscaleStatus}
    <dl class="status-grid">
      <div>
        <dt>{$_("Client detected")}</dt>
        <dd>
          <Tag
            type={tailscaleStatus.detected ? "green" : "warm-gray"}
            size="sm"
          >
            {tailscaleStatus.detected ? $_("Detected") : $_("Not detected")}
          </Tag>
        </dd>
      </div>
      <div>
        <dt>{$_("State")}</dt>
        <dd>
          <Tag type={tagType(tailscaleStatus.state)} size="sm">
            {tailscaleStatus.state || $_("Unknown")}
          </Tag>
        </dd>
      </div>
      <div>
        <dt>{$_("Backend state")}</dt>
        <dd>{tailscaleStatus.backend_state || $_("Unknown")}</dd>
      </div>
      <div>
        <dt>{$_("This node")}</dt>
        <dd>{tailscaleStatus.self_name || "—"}</dd>
      </div>
      <div>
        <dt>{$_("Last refreshed")}</dt>
        <dd>{formatDate(tailscaleStatus.last_refresh)}</dd>
      </div>
    </dl>
    {#if tailscaleStatus.message}
      <InlineNotification
        lowContrast
        hideCloseButton
        kind={tailscaleStatusKind(tailscaleStatus.state)}
        title={tailscaleStatusTitle(tailscaleStatus.state)}
        subtitle={tailscaleStatus.message}
      />
    {/if}
  {/if}

  <p class="tailscale-note">
    {$_(
      "Peer suggestions are hints only. A peer is linked only after you choose it and confirm the link on a device.",
    )}
  </p>
  <p class="tailscale-note">
    {$_(
      "Tailscale normally does not supply a hardware MAC address. Wake-on-LAN MAC addresses, when available, are informational and are not used to match or link devices.",
    )}
  </p>
  <div class="actions">
    <Button
      size="small"
      disabled={tailscaleLoading || tailscaleSaving || !tailscaleStatus}
      on:click={() => setTailscaleEnabled(!(tailscaleStatus?.enabled ?? false))}
    >
      {tailscaleSaving
        ? $_("Saving…")
        : tailscaleStatus?.enabled
        ? $_("Disable Tailscale")
        : $_("Enable Tailscale")}
    </Button>
    <Button
      size="small"
      kind="tertiary"
      disabled={tailscaleLoading || tailscaleSaving}
      on:click={loadTailscaleStatus}
    >
      {tailscaleLoading ? $_("Refreshing…") : $_("Refresh status")}
    </Button>
  </div>
</section>

<section class="diagnostics gs-card">
  <h3>{$_("Gateway diagnostics")}</h3>
  <p>
    {$_(
      "Check gateway health and create a support bundle that excludes credentials, private keys, image data, device identities, and browsing history.",
    )}
  </p>
  {#if diagnosticsError}
    <InlineNotification
      kind="error"
      title="Diagnostics error"
      subtitle={diagnosticsError}
      on:close={() => (diagnosticsError = "")}
    />
  {/if}
  <div class="actions">
    <Button
      size="small"
      disabled={loadingDiagnostics}
      on:click={loadDiagnostics}
      >{loadingDiagnostics ? $_("Checking…") : $_("Run diagnostics")}</Button
    >
    <Button size="small" kind="secondary" on:click={previewBundle}
      >{$_("Preview support bundle")}</Button
    >
    <Button
      size="small"
      kind="tertiary"
      disabled={!bundlePreview}
      on:click={downloadBundle}>{$_("Download preview")}</Button
    >
  </div>
  {#if diagnostics}
    <p class="overall">
      <strong>{$_("Overall status")}:</strong>
      <Tag type={tagType(diagnostics.overall)} size="sm"
        >{diagnostics.overall}</Tag
      >
    </p>
    <div class="checks">
      {#each diagnostics.checks || [] as check}
        <article>
          <header>
            <strong>{check.name}</strong><Tag
              type={tagType(check.status)}
              size="sm">{check.status}</Tag
            >
          </header>
          <p>{check.message}</p>
          {#if check.recovery}
            <p class="recovery">
              <strong>{$_("Recovery")}:</strong>
              {check.recovery}
            </p>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
  {#if bundlePreview}
    <label for="bundle-preview">{$_("Redacted bundle preview")}</label>
    <textarea id="bundle-preview" readonly value={bundlePreview} rows="16"
    ></textarea>
  {/if}
</section>

<style>
  .tailscale,
  .diagnostics {
    margin-top: 2rem;
    padding: 1.5rem;
  }
  .card-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }
  .card-heading p,
  .diagnostics > p {
    max-width: 48rem;
    margin: 0.5rem 0 1rem;
    color: var(--cds-text-secondary, #525252);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin: 1rem 0 1.25rem;
  }
  .status-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 1rem;
    margin: 1rem 0;
  }
  .status-grid div {
    min-width: 0;
  }
  .status-grid dt {
    color: var(--cds-text-secondary, #525252);
    font-size: 0.75rem;
    margin-bottom: 0.25rem;
  }
  .status-grid dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .tailscale-note {
    max-width: 48rem;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
    margin: 0.5rem 0;
  }
  .overall {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .checks {
    display: grid;
    gap: 0.75rem;
    margin: 1rem 0;
  }
  .checks article {
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
    padding: 1rem;
  }
  .checks header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    text-transform: capitalize;
  }
  .checks p {
    margin: 0.5rem 0 0;
  }
  .recovery {
    color: var(--cds-text-secondary, #525252);
  }
  label {
    display: block;
    font-weight: 600;
    margin: 1rem 0 0.5rem;
  }
  textarea {
    width: 100%;
    padding: 0.75rem;
    font-family: monospace;
    resize: vertical;
  }
</style>
