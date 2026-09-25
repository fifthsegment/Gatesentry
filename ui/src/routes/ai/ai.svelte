<script lang="ts">
  import {
    Accordion,
    AccordionItem,
    Breadcrumb,
    BreadcrumbItem,
    Button,
    InlineLoading,
    InlineNotification,
    Tag,
  } from "carbon-components-svelte";
  import { Renew } from "carbon-icons-svelte";
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import { store } from "../../store/apistore";

  type Probe = { state: string; detail?: string };
  type StatusSnapshot = {
    mode?: string;
    grok?: Probe;
    chatgpt?: Probe;
    local?: Probe;
    legacy?: Probe;
  };

  const providerOptions = [
    { value: "disabled", label: $_("Disable") },
    { value: "grok", label: $_("Enable Grok") },
    { value: "chatgpt", label: $_("Enable ChatGPT") },
    { value: "local", label: $_("Enable Local LLM") },
  ];

  const checking: Probe = { state: "checking", detail: "" };
  const notConfigured: Probe = { state: "not_configured", detail: "" };
  let status: StatusSnapshot | null = null;
  let probing = false;
  let statusError = "";
  let filterMode = "disabled";
  let hasGrokKey = false;
  let hasOpenAIKey = false;
  let hasLocalURL = false;
  let hasLegacyURL = false;

  function settingValue(json: any) {
    if (!json || typeof json !== "object") return "";
    const value = json.Value != null ? json.Value : json.value;
    return String(value || "").trim();
  }

  function nonempty(json: any) {
    if (json && typeof json === "object" && json.Configured != null) {
      return Boolean(json.Configured);
    }
    return settingValue(json) !== "";
  }

  async function loadConfigured() {
    const [mode, grok, openai, local, legacy] = await Promise.all([
      $store.api.getSetting("ai_image_filtering_mode"),
      $store.api.getSetting("ai_grok_api_key"),
      $store.api.getSetting("ai_openai_api_key"),
      $store.api.getSetting("ai_local_llm_url"),
      $store.api.getSetting("ai_scanner_url"),
    ]);
    filterMode = settingValue(mode) || "disabled";
    hasGrokKey = nonempty(grok);
    hasOpenAIKey = nonempty(openai);
    hasLocalURL = nonempty(local);
    hasLegacyURL = nonempty(legacy);
  }

  function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
    return Promise.race([
      promise,
      new Promise<T>((_, reject) =>
        setTimeout(() => reject(new Error($_("Status check timed out."))), ms),
      ),
    ]);
  }

  async function refreshStatus() {
    probing = true;
    statusError = "";
    try {
      await loadConfigured();
      const snapshot = await withTimeout<StatusSnapshot>(
        $store.api.doCall("/ai/status"),
        12000,
      );
      if (snapshot && typeof snapshot === "object") {
        status = snapshot;
        filterMode = snapshot.mode || filterMode;
      } else {
        throw new Error($_("The status response was not valid."));
      }
    } catch (error) {
      status = null;
      statusError =
        error instanceof Error && error.message
          ? error.message
          : $_("AI provider status could not be checked.");
    } finally {
      probing = false;
    }
  }

  function probeFor(
    key: keyof StatusSnapshot,
    snapshot: StatusSnapshot | null,
    inFlight: boolean,
    configured: boolean,
  ): Probe {
    const probe = snapshot?.[key];
    if (probe && typeof probe === "object" && "state" in probe) return probe;
    if (!configured) return notConfigured;
    if (inFlight) return checking;
    return { state: "unreachable", detail: $_("Status check did not complete") };
  }

  function selectedProviderProbe(
    snapshot: StatusSnapshot | null,
    mode: string,
    grok: Probe,
    chatgpt: Probe,
    local: Probe,
  ): Probe {
    const selectedMode = snapshot?.mode || mode || "disabled";
    if (selectedMode === "disabled") {
      return { state: "disabled", detail: $_("Image filtering is off") };
    }
    if (selectedMode === "grok") return grok;
    if (selectedMode === "chatgpt") return chatgpt;
    if (selectedMode === "local") return local;
    return notConfigured;
  }

  function providerLabel(mode: string) {
    if (mode === "grok") return $_("Grok");
    if (mode === "chatgpt") return $_("ChatGPT");
    if (mode === "local") return $_("Local LLM");
    return $_("Disabled");
  }

  function tagType(state: string) {
    switch (state) {
      case "ok":
        return "green";
      case "unauthorized":
        return "magenta";
      case "unreachable":
        return "red";
      case "checking":
        return "cool-gray";
      default:
        return "gray";
    }
  }

  function tagLabel(state: string) {
    switch (state) {
      case "ok":
        return $_("Reachable");
      case "unauthorized":
        return $_("Invalid key");
      case "unreachable":
        return $_("Unreachable");
      case "not_configured":
        return $_("Not configured");
      case "disabled":
        return $_("Disabled");
      case "checking":
        return $_("Checking…");
      default:
        return state;
    }
  }

  $: grokProbe = probeFor("grok", status, probing, hasGrokKey);
  $: chatgptProbe = probeFor("chatgpt", status, probing, hasOpenAIKey);
  $: localProbe = probeFor("local", status, probing, hasLocalURL);
  $: legacyProbe = probeFor("legacy", status, probing, hasLegacyURL);
  $: providerProbe = selectedProviderProbe(
    status,
    filterMode,
    grokProbe,
    chatgptProbe,
    localProbe,
  );

  onMount(refreshStatus);
</script>

<PageShell
  title={$_("AI image filtering")}
  description={$_(
    "Optionally classify image content seen through HTTPS inspection. External AI processing is disabled by default; images are sent only to the provider you select.",
  )}
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("AI image filtering")}</BreadcrumbItem>
    </Breadcrumb>
    <Tag type="red" size="sm">Alpha</Tag>
  </svelte:fragment>
  <svelte:fragment slot="actions">
    <Button
      kind="ghost"
      size="small"
      icon={Renew}
      disabled={probing}
      on:click={refreshStatus}
    >
      {probing ? $_("Checking…") : $_("Refresh status")}
    </Button>
  </svelte:fragment>

  {#if statusError}
    <InlineNotification
      kind="error"
      title={$_("AI status unavailable")}
      subtitle={statusError}
      on:close={() => (statusError = "")}
    />
  {/if}

  <section class="provider-panel" aria-labelledby="provider-title">
    <div class="provider-choice">
      <h3 id="provider-title">{$_("Provider")}</h3>
      <p>
        {$_(
          "Choose one provider. Selecting a hosted provider sends image content to that service for classification.",
        )}
      </p>
      <ConnectedSettingInput
        keyName="ai_image_filtering_mode"
        title={$_("AI image filtering provider")}
        labelText={$_("AI image filtering provider")}
        type="radio"
        helperText={$_("HTTPS inspection must be enabled before GateSentry can inspect image bytes.")}
        orientation="vertical"
        radioOptions={providerOptions}
        onSaved={async (value) => {
          filterMode = value;
          await refreshStatus();
        }}
      />
    </div>

    <aside class="provider-status" aria-labelledby="provider-status-title">
      <span class="eyebrow" id="provider-status-title">{$_("Selected provider status")}</span>
      {#if probing && !status}
        <InlineLoading description={$_("Checking provider…")} />
      {:else}
        <div class="status-line">
          <strong>{providerLabel(filterMode)}</strong>
          <Tag type={tagType(providerProbe.state)} size="sm">
            {tagLabel(providerProbe.state)}
          </Tag>
        </div>
        <p>{providerProbe.detail || $_("No additional status detail.")}</p>
      {/if}
    </aside>
  </section>

  {#if filterMode === "grok"}
    <section class="configuration" aria-labelledby="grok-configuration-title">
      <div class="section-head">
        <div>
          <h3 id="grok-configuration-title">{$_("Grok configuration")}</h3>
          <p>{$_("Credentials stay stored on this GateSentry server and are not returned to the browser.")}</p>
        </div>
        <Tag type={tagType(grokProbe.state)} size="sm">
          {tagLabel(grokProbe.state)}
        </Tag>
      </div>
      <div class="fields">
        <ConnectedSettingInput
          keyName="ai_grok_api_key"
          title={$_("Grok API key")}
          labelText={$_("Grok API key")}
          type="password"
          helperText={hasGrokKey
            ? $_("A key is saved. Enter a new value only to replace it.")
            : $_("Enter an xAI / Grok API key.")}
          onSaved={refreshStatus}
        />
        <ConnectedSettingInput
          keyName="ai_grok_model"
          title={$_("Grok model")}
          labelText={$_("Grok model")}
          type="text"
          helperText={$_("Leave empty to use the server default, grok-4.5.")}
          onSaved={refreshStatus}
        />
      </div>
    </section>
  {:else if filterMode === "chatgpt"}
    <section class="configuration" aria-labelledby="chatgpt-configuration-title">
      <div class="section-head">
        <div>
          <h3 id="chatgpt-configuration-title">{$_("ChatGPT configuration")}</h3>
          <p>{$_("Credentials stay stored on this GateSentry server and are not returned to the browser.")}</p>
        </div>
        <Tag type={tagType(chatgptProbe.state)} size="sm">
          {tagLabel(chatgptProbe.state)}
        </Tag>
      </div>
      <div class="fields">
        <ConnectedSettingInput
          keyName="ai_openai_api_key"
          title={$_("OpenAI API key")}
          labelText={$_("OpenAI API key")}
          type="password"
          helperText={hasOpenAIKey
            ? $_("A key is saved. Enter a new value only to replace it.")
            : $_("Enter an OpenAI API key.")}
          onSaved={refreshStatus}
        />
        <ConnectedSettingInput
          keyName="ai_openai_model"
          title={$_("ChatGPT model")}
          labelText={$_("ChatGPT model")}
          type="text"
          helperText={$_("Leave empty to use the server default, gpt-4o-mini.")}
          onSaved={refreshStatus}
        />
      </div>
    </section>
  {:else if filterMode === "local"}
    <section class="configuration" aria-labelledby="local-configuration-title">
      <div class="section-head">
        <div>
          <h3 id="local-configuration-title">{$_("Local LLM configuration")}</h3>
          <p>{$_("Point GateSentry at an Ollama-compatible HTTP API on a network you trust.")}</p>
        </div>
        <Tag type={tagType(localProbe.state)} size="sm">
          {tagLabel(localProbe.state)}
        </Tag>
      </div>
      <div class="fields">
        <ConnectedSettingInput
          keyName="ai_local_llm_url"
          title={$_("Local LLM URL")}
          labelText={$_("Local LLM URL")}
          type="text"
          helperText={$_("For example, http://127.0.0.1:11434")}
          onSaved={refreshStatus}
        />
        <ConnectedSettingInput
          keyName="ai_local_llm_model"
          title={$_("Ollama model")}
          labelText={$_("Ollama model")}
          type="text"
          helperText={$_("Leave empty to use the server default, llava.")}
          onSaved={refreshStatus}
        />
      </div>
    </section>
  {:else}
    <section class="disabled-state" aria-labelledby="disabled-title">
      <h3 id="disabled-title">{$_("AI image filtering is disabled")}</h3>
      <p>{$_("No image content is sent to Grok, ChatGPT, or a local LLM.")}</p>
    </section>
  {/if}

  <section class="advanced" aria-labelledby="advanced-title">
    <h3 id="advanced-title">{$_("Advanced")}</h3>
    <Accordion align="start">
      <AccordionItem title={$_("Legacy local scanner")}>
        <div class="advanced-content">
          <div class="section-head">
            <p>
              {$_(
                "Optional compatibility path for the original local classifier endpoint. It is separate from the selected provider above.",
              )}
            </p>
            <Tag type={tagType(legacyProbe.state)} size="sm">
              {tagLabel(legacyProbe.state)}
            </Tag>
          </div>
          {#if legacyProbe.detail}
            <p class="status-detail">{legacyProbe.detail}</p>
          {/if}
          <ConnectedSettingInput
            keyName="ai_scanner_url"
            title={$_("AI service URL")}
            labelText={$_("AI service URL")}
            type="text"
            helperText={$_("HTTP endpoint that accepts a multipart image upload. Leave empty unless you run the legacy scanner.")}
            onSaved={refreshStatus}
          />
        </div>
      </AccordionItem>
    </Accordion>
  </section>
</PageShell>

<style>
  .section-head,
  .status-line {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .provider-choice > p,
  .section-head p,
  .disabled-state p,
  .provider-status p,
  .advanced-content > p {
    max-width: 48rem;
    margin: 0.5rem 0 0;
    color: var(--cds-text-secondary, #525252);
    line-height: 1.45;
  }

  .provider-panel {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(16rem, 24rem);
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .provider-choice,
  .provider-status,
  .configuration,
  .disabled-state {
    padding: 1.5rem;
    background: var(--cds-ui-01, #f4f4f4);
  }

  .provider-choice :global(.connected-setting) {
    margin-top: 1.25rem;
  }

  .provider-status {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    border-left: 1px solid var(--cds-border-subtle, #e0e0e0);
    background: var(--cds-ui-02, #ffffff);
  }

  .eyebrow {
    color: var(--cds-text-secondary, #525252);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .status-line {
    align-items: center;
  }

  .configuration,
  .disabled-state {
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .fields {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(18rem, 1fr));
    gap: 1.25rem;
    margin-top: 1.25rem;
  }

  .advanced h3 {
    margin-bottom: 0.75rem;
  }

  .advanced-content {
    display: grid;
    gap: 1rem;
    padding: 0.5rem 0;
  }

  .status-detail {
    font-size: 0.875rem;
  }

  @media (max-width: 48rem) {
    .provider-panel {
      grid-template-columns: 1fr;
    }

    .provider-status {
      border-left: 0;
      border-top: 1px solid var(--cds-border-subtle, #e0e0e0);
    }
  }
</style>
