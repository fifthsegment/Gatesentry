<script lang="ts">
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";
  import { Tag } from "carbon-components-svelte";
  import { MachineLearning } from "carbon-icons-svelte";

  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import { store } from "../../store/apistore";

  const providerOptions = [
    { value: "disabled", label: $_("Disable") },
    { value: "grok", label: $_("Enable Grok") },
    { value: "chatgpt", label: $_("Enable ChatGPT") },
    { value: "local", label: $_("Enable Local LLM") },
  ];

  const checking = { state: "checking", detail: "" };
  const notConfigured = { state: "not_configured", detail: "" };
  let status = null;
  let probing = false;
  let filterMode = "disabled";
  let hasGrokKey = false;
  let hasOpenAIKey = false;
  let hasLocalURL = false;
  let hasLegacyURL = false;

  function settingValue(json) {
    if (!json || typeof json !== "object") {
      return "";
    }
    const v = json.Value != null ? json.Value : json.value;
    return String(v || "").trim();
  }

  function nonempty(json) {
    return settingValue(json) !== "";
  }

  async function loadConfigured() {
    try {
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
    } catch {
      /* leave previous flags */
    }
  }

  function withTimeout(promise, ms) {
    return Promise.race([
      promise,
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error("status timeout")), ms),
      ),
    ]);
  }

  async function refreshStatus() {
    probing = true;
    try {
      await loadConfigured();
      const json = await withTimeout($store.api.doCall("/ai/status"), 12000);
      if (json && json.grok) {
        status = json;
        if (json.mode) {
          filterMode = json.mode;
        }
      }
    } catch {
      status = null;
    } finally {
      probing = false;
    }
  }

  function probeFor(key, snapshot, inFlight, configured) {
    if (snapshot && snapshot[key] && snapshot[key].state) {
      return snapshot[key];
    }
    if (!configured) {
      return notConfigured;
    }
    if (inFlight) {
      return checking;
    }
    return { state: "unreachable", detail: "Status check did not complete" };
  }

  function selectedProviderProbe(snapshot, mode, grok, chatgpt, local) {
    const m = (snapshot && snapshot.mode) || mode || "disabled";
    if (m === "disabled") {
      return { state: "disabled", detail: "Image filtering is off" };
    }
    if (m === "grok") {
      return grok;
    }
    if (m === "chatgpt") {
      return chatgpt;
    }
    if (m === "local") {
      return local;
    }
    return notConfigured;
  }

  // Pass lets as args so the template invalidates when probes finish.
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

  function tagType(state) {
    switch (state) {
      case "ok":
        return "green";
      case "unauthorized":
        return "magenta";
      case "unreachable":
        return "red";
      case "disabled":
      case "not_configured":
        return "gray";
      case "checking":
        return "cool-gray";
      default:
        return "gray";
    }
  }

  function tagLabel(state) {
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

  onMount(async () => {
    await loadConfigured();
    await refreshStatus();
  });
</script>

<div class="gs-page-title">
  <MachineLearning size={24} />
  <h2>Gatesentry AI</h2>
  <Tag type="red" size="sm">Alpha</Tag>
</div>
<p class="gs-page-info">
  {$_("Work in progress. Remote vision APIs are too slow to block every image on the request path; scanning strategy is still being decided.")}
</p>

<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <h5>{$_("Image filtering")}</h5>
      <span class="card-status" title={providerProbe.detail}>
        <Tag type={tagType(providerProbe.state)} size="sm"
          >{tagLabel(providerProbe.state)}</Tag
        >
      </span>
    </div>
    <p class="card-copy">
      {$_("Use a vision model to classify images that pass through the MITM proxy (NSFW / not safe for work). HTTPS inspection must be enabled or the proxy never sees the image bytes.")}
    </p>
    <ConnectedSettingInput
      keyName="ai_image_filtering_mode"
      title={$_("AI image filtering")}
      labelText={$_("AI image filtering")}
      type="radio"
      helperText=""
      orientation="horizontal"
      radioOptions={providerOptions}
      onSaved={refreshStatus}
    />
  </div>
</section>

<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <h5>{$_("API keys")}</h5>
      <span class="card-status">
        <Tag
          type={tagType(grokProbe.state)}
          size="sm"
          title={grokProbe.detail}>Grok: {tagLabel(grokProbe.state)}</Tag
        >
        <Tag
          type={tagType(chatgptProbe.state)}
          size="sm"
          title={chatgptProbe.detail}
          >ChatGPT: {tagLabel(chatgptProbe.state)}</Tag
        >
      </span>
    </div>
    <p class="card-copy">
      {$_("Keys are stored in GateSentry settings on the server. They are never sent to the browser except to this admin page. Use the provider that matches the radio above.")}
    </p>
    <div class="card-fields">
      <ConnectedSettingInput
        keyName="ai_grok_api_key"
        title={$_("Grok API key")}
        labelText={$_("Grok API key")}
        type="password"
        helperText={$_("xAI / Grok key from console.x.ai. Used when Enable Grok is selected.")}
        onSaved={refreshStatus}
      />
      <ConnectedSettingInput
        keyName="ai_grok_model"
        title={$_("Grok model")}
        labelText={$_("Grok model")}
        type="text"
        helperText={$_("Vision model id. Empty uses grok-4.5.")}
        onSaved={refreshStatus}
      />
      <ConnectedSettingInput
        keyName="ai_openai_api_key"
        title={$_("OpenAI API key")}
        labelText={$_("OpenAI API key")}
        type="password"
        helperText={$_("OpenAI key. Used when Enable ChatGPT is selected.")}
        onSaved={refreshStatus}
      />
      <ConnectedSettingInput
        keyName="ai_openai_model"
        title={$_("ChatGPT model")}
        labelText={$_("ChatGPT model")}
        type="text"
        helperText={$_("Vision model id. Empty uses gpt-4o-mini.")}
        onSaved={refreshStatus}
      />
    </div>
  </div>
</section>

<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <h5>{$_("Local LLM (Ollama)")}</h5>
      <span class="card-status" title={localProbe.detail}>
        <Tag type={tagType(localProbe.state)} size="sm"
          >{tagLabel(localProbe.state)}</Tag
        >
      </span>
    </div>
    <p class="card-copy">
      {$_("Used when Enable Local LLM is selected. Point this at an Ollama HTTP API. Scanning is not live in the proxy yet (Alpha).")}
    </p>
    <div class="card-fields">
      <ConnectedSettingInput
        keyName="ai_local_llm_url"
        title={$_("Local LLM URL")}
        labelText={$_("Local LLM URL")}
        type="text"
        helperText={$_("Ollama base URL, for example http://127.0.0.1:11434")}
        onSaved={refreshStatus}
      />
      <ConnectedSettingInput
        keyName="ai_local_llm_model"
        title={$_("Ollama model")}
        labelText={$_("Ollama model")}
        type="text"
        helperText={$_("Vision model tag, for example gemma4:e4b-mlx. Empty uses llava.")}
        onSaved={refreshStatus}
      />
    </div>
  </div>
</section>

<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <h5>{$_("Legacy local scanner")}</h5>
      <span class="card-status" title={legacyProbe.detail}>
        <Tag type={tagType(legacyProbe.state)} size="sm"
          >{tagLabel(legacyProbe.state)}</Tag
        >
      </span>
    </div>
    <p class="card-copy">
      {$_("Optional. The original GateSentry AI path posted image bytes to a local classifier URL. It is not used when Grok or ChatGPT is selected.")}
    </p>
    <ConnectedSettingInput
      keyName="ai_scanner_url"
      title={$_("AI Service URL")}
      labelText={$_("AI Service URL")}
      type="text"
      helperText={$_("HTTP endpoint that accepts a multipart image upload. Leave empty unless you run a local scanner.")}
      onSaved={refreshStatus}
    />
  </div>
</section>

<style>
  .card-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
  }
  .card-header h5 {
    margin: 0;
    font-weight: 600;
  }
  .card-status {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .card-copy {
    margin: 0 0 1rem 0;
    font-size: 0.875rem;
    color: #525252;
    line-height: 1.45;
    max-width: 46rem;
  }
  .card-fields {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .settings-card :global(.bx--radio-button-wrapper) {
    margin-right: 1.25rem;
    margin-bottom: 0.5rem;
  }
  .settings-card :global(.bx--radio-button-group) {
    flex-wrap: wrap;
  }
</style>
