<script lang="ts">
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    InlineLoading,
    InlineNotification,
    Tab,
    TabContent,
    Tabs,
    Tag,
    TextInput,
  } from "carbon-components-svelte";

  import ConnectedCertificateComposed from "../../components/connectedCertificateComposed.svelte";
  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
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

  interface DiagnosticCheck {
    name: string;
    status: string;
    message: string;
    recovery?: string;
  }

  interface Diagnostics {
    overall: string;
    checks?: DiagnosticCheck[];
  }

  let diagnostics: Diagnostics | null = null;
  let bundlePreview = "";
  let loadingDiagnostics = false;
  let loadingBundle = false;
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
    loadingBundle = true;
    diagnosticsError = "";
    try {
      const bundle = await $store.api.doCall("/diagnostics/bundle");
      bundlePreview = JSON.stringify(bundle, null, 2);
    } catch (error) {
      diagnosticsError =
        error instanceof Error ? error.message : "Bundle preview failed";
    } finally {
      loadingBundle = false;
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

<PageShell
  title={$_("Settings")}
  description={$_(
    "Configure this gateway, HTTPS inspection, network integrations, and support diagnostics.",
  )}
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("Settings")}</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <Tabs
    type="container"
    autoWidth
    iconDescription={$_("Show settings sections")}
  >
    <Tab label={$_("General")} />
    <Tab label={$_("HTTPS inspection")} />
    <Tab label={$_("Connectivity")} />
    <Tab label={$_("Diagnostics")} />

    <svelte:fragment slot="content">
      <TabContent>
        <div class="tab-panel">
          <section class="settings-card" aria-labelledby="gateway-heading">
            <div class="section-heading">
              <h3 id="gateway-heading">{$_("Gateway")}</h3>
              <p>
                {$_(
                  "Choose where GateSentry stores its local decision log and review the signed-in administrator.",
                )}
              </p>
            </div>

            <div class="setting-row">
              <div class="setting-copy">
                <h4>{$_("Decision log storage")}</h4>
                <p>
                  {$_(
                    "GateSentry writes filtering decisions to this SQLite database path.",
                  )}
                </p>
              </div>
              <div class="setting-control">
                <ConnectedGeneralSettingInput
                  keyName="log_location"
                  title={$_("Log location")}
                  labelText={$_("Log location")}
                  type="text"
                  helperText={$_(
                    "Restart GateSentry after changing this path.",
                  )}
                />
              </div>
            </div>

            <InlineNotification
              lowContrast
              hideCloseButton
              kind="info"
              title={$_("Raspberry Pi storage guidance")}
              subtitle={$_(
                'To reduce SD card writes, set the log location to "/tmp/log.db". This keeps the log in RAM, so history is cleared whenever the device restarts.',
              )}
            />

            <div class="setting-row">
              <div class="setting-copy">
                <h4>{$_("Administrator")}</h4>
                <p>
                  {$_(
                    "This identity comes from the authenticated session and cannot be edited here.",
                  )}
                </p>
              </div>
              <div class="setting-control">
                <TextInput
                  value={$store.api.username}
                  type="text"
                  title={$_("Admin username")}
                  labelText={$_("Admin username")}
                  helperText={$_("Change the password from the account menu.")}
                  disabled
                />
              </div>
            </div>
          </section>
        </div>
      </TabContent>

      <TabContent>
        <div class="tab-panel">
          <section class="settings-card" aria-labelledby="https-heading">
            <div class="section-heading">
              <h3 id="https-heading">{$_("HTTPS inspection")}</h3>
              <p>
                {$_(
                  "Advanced, opt-in inspection for encrypted web traffic. Devices must trust the GateSentry certificate.",
                )}
              </p>
            </div>
            <div class="setting-row">
              <div class="setting-copy">
                <h4>{$_("Inspection status")}</h4>
                <p>
                  {$_(
                    "Changing this setting affects proxy traffic; DNS filtering remains separate.",
                  )}
                </p>
              </div>
              <div class="setting-control"><HttpsToggle /></div>
            </div>
          </section>

          <section class="settings-card certificate-card">
            <ConnectedCertificateComposed />
          </section>
        </div>
      </TabContent>

      <TabContent>
        <div class="tab-panel">
          <section class="settings-card" aria-labelledby="tailscale-heading">
            <div class="card-heading">
              <div class="section-heading">
                <h3 id="tailscale-heading">{$_("Tailscale")}</h3>
                <p>
                  {$_(
                    "Discover Tailscale peers and explicitly link them to devices in the device inventory.",
                  )}
                </p>
              </div>
              {#if tailscaleStatus}
                <Tag
                  type={tailscaleStatus.enabled ? "green" : "gray"}
                  size="sm"
                >
                  {tailscaleStatus.enabled ? $_("Enabled") : $_("Disabled")}
                </Tag>
              {/if}
            </div>

            {#if tailscaleError}
              <InlineNotification
                kind="error"
                title={$_("Tailscale error")}
                subtitle={tailscaleError}
                on:close={() => (tailscaleError = "")}
              />
            {/if}
            {#if tailscaleNotice}
              <InlineNotification
                kind="success"
                title={$_("Tailscale updated")}
                subtitle={tailscaleNotice}
                on:close={() => (tailscaleNotice = "")}
              />
            {/if}

            {#if tailscaleLoading && !tailscaleStatus}
              <InlineLoading description={$_("Checking Tailscale…")} />
            {:else if tailscaleStatus}
              <dl class="status-grid">
                <div>
                  <dt>{$_("Client detected")}</dt>
                  <dd>
                    <Tag
                      type={tailscaleStatus.detected ? "green" : "warm-gray"}
                      size="sm"
                    >
                      {tailscaleStatus.detected
                        ? $_("Detected")
                        : $_("Not detected")}
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

            <div class="guidance">
              <p>
                {$_(
                  "Peer suggestions are hints only. A peer is linked only after you choose it and confirm the link on a device.",
                )}
              </p>
              <p>
                {$_(
                  "Tailscale normally does not supply a hardware MAC address. Wake-on-LAN MAC addresses, when available, are informational and are not used to match or link devices.",
                )}
              </p>
            </div>

            <div class="actions">
              <Button
                size="small"
                disabled={tailscaleLoading ||
                  tailscaleSaving ||
                  !tailscaleStatus}
                on:click={() =>
                  setTailscaleEnabled(!(tailscaleStatus?.enabled ?? false))}
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
        </div>
      </TabContent>

      <TabContent>
        <div class="tab-panel">
          <section class="settings-card" aria-labelledby="diagnostics-heading">
            <div class="section-heading">
              <h3 id="diagnostics-heading">{$_("Gateway diagnostics")}</h3>
              <p>
                {$_(
                  "Check gateway health and create a support bundle that excludes credentials, private keys, image data, device identities, and browsing history.",
                )}
              </p>
            </div>

            {#if diagnosticsError}
              <InlineNotification
                kind="error"
                title={$_("Diagnostics error")}
                subtitle={diagnosticsError}
                on:close={() => (diagnosticsError = "")}
              />
            {/if}

            <div class="actions diagnostics-actions">
              <Button
                size="small"
                disabled={loadingDiagnostics}
                on:click={loadDiagnostics}
              >
                {loadingDiagnostics ? $_("Checking…") : $_("Run diagnostics")}
              </Button>
              <Button
                size="small"
                kind="secondary"
                disabled={loadingBundle}
                on:click={previewBundle}
              >
                {loadingBundle
                  ? $_("Preparing…")
                  : $_("Preview support bundle")}
              </Button>
              <Button
                size="small"
                kind="tertiary"
                disabled={!bundlePreview || loadingBundle}
                on:click={downloadBundle}>{$_("Download preview")}</Button
              >
            </div>

            {#if loadingDiagnostics && !diagnostics}
              <InlineLoading description={$_("Running gateway checks…")} />
            {:else if diagnostics}
              <div class="diagnostic-summary">
                <span>{$_("Overall status")}</span>
                <Tag type={tagType(diagnostics.overall)} size="sm">
                  {diagnostics.overall}
                </Tag>
              </div>
              <ul class="checks">
                {#each diagnostics.checks || [] as check}
                  <li>
                    <div class="check-heading">
                      <strong>{check.name}</strong>
                      <Tag type={tagType(check.status)} size="sm">
                        {check.status}
                      </Tag>
                    </div>
                    <p>{check.message}</p>
                    {#if check.recovery}
                      <p class="recovery">
                        <strong>{$_("Recovery")}:</strong>
                        {check.recovery}
                      </p>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}

            {#if bundlePreview}
              <div class="bundle-preview">
                <div>
                  <h4 id="bundle-preview-label">
                    {$_("Redacted support bundle")}
                  </h4>
                  <p>
                    {$_(
                      "Read-only JSON. Review it before downloading or sharing it with support.",
                    )}
                  </p>
                </div>
                <pre aria-labelledby="bundle-preview-label"><code
                    >{bundlePreview}</code
                  ></pre>
              </div>
            {/if}
          </section>
        </div>
      </TabContent>
    </svelte:fragment>
  </Tabs>
</PageShell>

<style>
  .section-heading h3,
  .setting-copy h4,
  .bundle-preview h4 {
    margin: 0;
  }

  .section-heading p,
  .setting-copy p,
  .bundle-preview p {
    max-width: 48rem;
    margin: 0.5rem 0 0;
    color: var(--cds-text-secondary, #525252);
    line-height: 1.4;
  }

  .tab-panel {
    display: grid;
    gap: 1rem;
    min-width: 0;
    padding-top: 1rem;
  }

  .settings-card {
    min-width: 0;
    padding: 1.5rem;
    background: var(--cds-layer-01, #fff);
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .certificate-card {
    padding-top: 1.25rem;
  }

  .card-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  .setting-row {
    display: grid;
    grid-template-columns: minmax(14rem, 0.8fr) minmax(18rem, 1.2fr);
    gap: 2rem;
    align-items: start;
    padding: 1.5rem 0;
    border-bottom: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .setting-row:last-child {
    padding-bottom: 0;
    border-bottom: 0;
  }

  .setting-control,
  .setting-copy,
  .section-heading {
    min-width: 0;
  }

  .status-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
    gap: 0;
    margin: 1.25rem 0;
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .status-grid div {
    min-width: 0;
    padding: 1rem;
    border-right: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .status-grid div:last-child {
    border-right: 0;
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

  .guidance {
    max-width: 48rem;
    margin-top: 1rem;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
    line-height: 1.4;
  }

  .guidance p {
    margin: 0.5rem 0 0;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 1.25rem;
  }

  .diagnostics-actions {
    margin-bottom: 1rem;
  }

  .diagnostic-summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
    background: var(--cds-layer-02, #f4f4f4);
    font-weight: 600;
  }

  .checks {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1px;
    padding: 1px;
    margin: 1px 0 0;
    list-style: none;
    background: var(--cds-border-subtle, #e0e0e0);
  }

  .checks li {
    min-width: 0;
    padding: 1rem;
    background: var(--cds-layer-01, #fff);
  }

  .check-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    text-transform: capitalize;
  }

  .checks p {
    margin: 0.5rem 0 0;
    overflow-wrap: anywhere;
  }

  .recovery {
    color: var(--cds-text-secondary, #525252);
  }

  .bundle-preview {
    min-width: 0;
    margin-top: 1.5rem;
  }

  .bundle-preview pre {
    box-sizing: border-box;
    width: 100%;
    max-height: 24rem;
    margin: 0.75rem 0 0;
    padding: 1rem;
    overflow: auto;
    background: var(--cds-layer-02, #f4f4f4);
    border-left: 0.25rem solid var(--cds-border-interactive, #0f62fe);
    color: var(--cds-text-primary, #161616);
    font:
      0.75rem/1.45 "IBM Plex Mono",
      monospace;
    white-space: pre;
  }

  :global(.page-shell__content > .bx--tabs),
  :global(.page-shell__content > .bx--tabs .bx--tabs__nav),
  :global(.page-shell__content > .bx--tabs [role="tabpanel"]) {
    max-width: 100%;
    min-width: 0;
  }

  @media (max-width: 66rem) {
    .setting-row {
      grid-template-columns: minmax(12rem, 0.75fr) minmax(16rem, 1.25fr);
      gap: 1.25rem;
    }

    .checks {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  @media (max-width: 42rem) {
    .settings-card {
      padding: 1rem;
    }

    .card-heading,
    .setting-row {
      grid-template-columns: minmax(0, 1fr);
      flex-direction: column;
    }

    .setting-row {
      gap: 1rem;
    }

    .status-grid {
      grid-template-columns: minmax(0, 1fr);
    }

    .status-grid div {
      border-right: 0;
      border-bottom: 1px solid var(--cds-border-subtle, #e0e0e0);
    }

    .status-grid div:last-child {
      border-bottom: 0;
    }

    .actions :global(.bx--btn) {
      max-width: none;
      width: 100%;
    }
  }
</style>
