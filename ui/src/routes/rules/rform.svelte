<script lang="ts">
  import {
    Button,
    ComboBox,
    Dropdown,
    InlineLoading,
    InlineNotification,
    MultiSelect,
    Tag,
    TextArea,
    TextInput,
    Toggle,
  } from "carbon-components-svelte";
  import { ArrowLeft, TrashCan, Save } from "carbon-icons-svelte";
  import { onMount, createEventDispatcher } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import { MIME_TYPE_ITEMS } from "../../lib/mimetypes";
  import { notificationstore } from "../../store/notifications";

  export let rule = {
    id: "",
    name: "",
    enabled: true,
    priority: 0,
    domain: "",
    action: "allow",
    mitm_action: "default",
    blocked_content_types: [],
    url_regex_patterns: [],
    domain_patterns: [],
    domain_lists: [],
    content_domain_lists: [],
    keyword_filter_enabled: false,
    time_restriction: { from: "00:00", to: "23:59" },
    users: [],
    description: "",
  };

  export let isNew = false;

  const dispatch = createEventDispatcher();
  const API_BASE = getBasePath() + "/api/rules";

  let saving = false;
  let deleting = false;
  let error = "";
  let validationError = "";

  // Form inputs
  let domainPatternInput = "";
  let contentTypeInput = "";
  let urlRegexInput = "";
  let userInput = "";
  let contentTypeSelectedId = undefined;
  let userSelectedId = undefined;

  // API data
  let availableDomainLists = [];
  let availableUsers = [];
  let globalMitmEnabled = false;

  // Resolve effective MITM state: enable → true, disable → false, default → globalMitmEnabled
  $: mitmEffective =
    rule.mitm_action === "enable"
      ? true
      : rule.mitm_action === "disable"
      ? false
      : globalMitmEnabled;

  // Ensure arrays and objects are initialized
  $: {
    if (!rule.blocked_content_types) rule.blocked_content_types = [];
    if (!rule.url_regex_patterns) rule.url_regex_patterns = [];
    if (!rule.users) rule.users = [];
    if (!rule.domain_patterns) rule.domain_patterns = [];
    if (!rule.domain_lists) rule.domain_lists = [];
    if (!rule.content_domain_lists) rule.content_domain_lists = [];
    if (!rule.time_restriction)
      rule.time_restriction = { from: "00:00", to: "23:59" };
    if (rule.domain && rule.domain.trim()) {
      if (!rule.domain_patterns.includes(rule.domain.trim())) {
        rule.domain_patterns = [...rule.domain_patterns, rule.domain.trim()];
      }
      rule.domain = "";
    }
  }

  onMount(async () => {
    try {
      const token = localStorage.getItem("jwt");
      const [listsRes, usersRes, mitmRes] = await Promise.all([
        fetch(getBasePath() + "/api/domainlists", {
          headers: { Authorization: `Bearer ${token}` },
        }),
        fetch(getBasePath() + "/api/users", {
          headers: { Authorization: `Bearer ${token}` },
        }),
        fetch(getBasePath() + "/api/settings/enable_https_filtering", {
          headers: { Authorization: `Bearer ${token}` },
        }),
      ]);
      if (listsRes.ok) {
        const data = await listsRes.json();
        availableDomainLists = (data.lists || []).map((l) => ({
          id: l.id,
          text: `${l.name} (${l.entry_count || 0})`,
        }));
      }
      if (usersRes.ok) {
        const data = await usersRes.json();
        availableUsers = (data.users || []).map((u) => ({
          id: u.username,
          text: u.username,
        }));
      }
      if (mitmRes.ok) {
        const data = await mitmRes.json();
        globalMitmEnabled = data.Value === "true";
      }
    } catch (e) {
      console.error("Failed to fetch data:", e);
    }
  });

  // --- Domain Patterns ---
  function addDomainPattern() {
    if (domainPatternInput.trim()) {
      rule.domain_patterns = [
        ...rule.domain_patterns,
        domainPatternInput.trim(),
      ];
      domainPatternInput = "";
    }
  }
  function removeDomainPattern(pattern) {
    rule.domain_patterns = rule.domain_patterns.filter((p) => p !== pattern);
  }

  // --- Content Types ---
  function addContentType(value?: string) {
    const v = (value || contentTypeInput || "").trim();
    if (v && !rule.blocked_content_types.includes(v)) {
      rule.blocked_content_types = [...rule.blocked_content_types, v];
    }
    contentTypeInput = "";
    contentTypeSelectedId = undefined;
  }
  function shouldFilterMimeItem(item, value) {
    if (!value) return true;
    return item.text.toLowerCase().includes(value.toLowerCase());
  }
  function removeContentType(type) {
    rule.blocked_content_types = rule.blocked_content_types.filter(
      (t) => t !== type,
    );
  }

  // --- URL Regex ---
  function addUrlRegex() {
    if (urlRegexInput.trim()) {
      rule.url_regex_patterns = [
        ...rule.url_regex_patterns,
        urlRegexInput.trim(),
      ];
      urlRegexInput = "";
    }
  }
  function removeUrlRegex(pattern) {
    rule.url_regex_patterns = rule.url_regex_patterns.filter(
      (p) => p !== pattern,
    );
  }

  // --- Users ---
  function addUser(value?: string) {
    const v = (value || userInput || "").trim();
    if (v && !rule.users.includes(v)) {
      rule.users = [...rule.users, v];
    }
    userInput = "";
    userSelectedId = undefined;
  }
  function shouldFilterUserItem(item, value) {
    if (!value) return true;
    return item.text.toLowerCase().includes(value.toLowerCase());
  }
  function removeUser(user) {
    rule.users = rule.users.filter((u) => u !== user);
  }

  // --- Save ---
  async function saveRule() {
    validationError = "";
    if (rule.enabled && rule.time_restriction) {
      const from = rule.time_restriction.from || "00:00";
      const to = rule.time_restriction.to || "23:59";
      if (to <= from) {
        validationError = "End time must be after begin time.";
        return;
      }
    }

    saving = true;
    error = "";
    try {
      const token = localStorage.getItem("jwt");
      const url = rule.id ? `${API_BASE}/${rule.id}` : API_BASE;
      const method = rule.id ? "PUT" : "POST";
      const response = await fetch(url, {
        method,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(rule),
      });
      if (!response.ok) {
        const errorData = await response.text();
        throw new Error(`Failed to save rule: ${errorData}`);
      }
      notificationstore.add({
        title: "Saved",
        subtitle: `Rule "${rule.name || "Unnamed"}" saved successfully`,
        kind: "success",
        timeout: 3000,
      });
      dispatch("back");
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  // --- Delete ---
  async function deleteRule() {
    if (!rule.id) {
      dispatch("back");
      return;
    }
    deleting = true;
    error = "";
    try {
      const token = localStorage.getItem("jwt");
      const response = await fetch(`${API_BASE}/${rule.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok) {
        const errorData = await response.text();
        throw new Error(`Failed to delete rule: ${errorData}`);
      }
      notificationstore.add({
        title: "Deleted",
        subtitle: `Rule "${rule.name || "Unnamed"}" deleted`,
        kind: "success",
        timeout: 3000,
      });
      dispatch("back");
    } catch (err) {
      error = err.message;
    } finally {
      deleting = false;
    }
  }

  function goBack() {
    dispatch("back");
  }

  // --- Rule Tester ---
  let testUrl = "";
  let testResult = null;
  let testLive = false;
  let testLoading = false;
  let testUser = "";

  async function testRule() {
    testResult = null;
    if (!testUrl.trim()) return;

    testLoading = true;
    try {
      const token = localStorage.getItem("jwt");
      const res = await fetch(getBasePath() + "/api/test/rule-match", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          rule: rule,
          url: testUrl.trim(),
          user: testUser.trim(),
          live: testLive,
        }),
      });

      if (!res.ok) {
        const errText = await res.text();
        testResult = { error: `Server error: ${errText}` };
        return;
      }

      testResult = await res.json();
    } catch (err) {
      testResult = { error: `Request failed: ${err.message}` };
    } finally {
      testLoading = false;
    }
  }
</script>

<!-- Header bar -->
<div class="rd-header">
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <span class="rd-back" on:click={goBack} title="Back to rules">
    <ArrowLeft size={20} />
  </span>
  <div class="rd-header-text">
    <span class="rd-header-label">{isNew ? "New Rule" : "Edit Rule"}</span>
  </div>
</div>

{#if error}
  <InlineNotification
    kind="error"
    title="Error"
    subtitle={error}
    on:close={() => (error = "")}
  />
{/if}

<!-- Rule Definition -->
<div class="rd-section-header">
  <span class="rd-section-title">Rule Definition</span>
</div>

<p class="rd-filter-hint">Rule name and schedule.</p>

<div class="rd-scroll">
  <!-- Name -->
  <div class="rd-field">
    <span class="rd-field-label">Name</span>
    <TextInput
      size="sm"
      hideLabel
      bind:value={rule.name}
      placeholder="Rule Name"
    />
  </div>

  <!-- Rule Status -->
  <div class="rd-field">
    <div class="rd-toggle-card">
      <div class="rd-toggle-info">
        <span class="rd-field-label" style="margin-bottom:0">Rule Status</span>
        <span class="rd-toggle-desc"
          >{rule.enabled
            ? "This rule is active and being evaluated"
            : "This rule is disabled and will be skipped"}</span
        >
      </div>
      <Toggle
        size="sm"
        bind:toggled={rule.enabled}
        hideLabel
        labelA=""
        labelB=""
      />
    </div>
  </div>

  <!-- Active Hours -->
  {#if rule.enabled}
    <div class="rd-field">
      <span class="rd-field-label">Active Hours</span>
      <div class="rd-time-inputs">
        <input
          type="time"
          class="rd-time"
          bind:value={rule.time_restriction.from}
        />
        <span class="rd-time-sep">to</span>
        <input
          type="time"
          class="rd-time"
          bind:value={rule.time_restriction.to}
        />
      </div>
      {#if validationError}
        <p class="rd-val-error">{validationError}</p>
      {/if}
    </div>
  {/if}

  <!-- SSL Inspection -->
  <div class="rd-field">
    <span class="rd-field-label">SSL Inspection (MITM)</span>
    <Dropdown
      size="sm"
      titleText=""
      selectedId={rule.mitm_action}
      on:select={(e) => {
        rule.mitm_action = e.detail.selectedId;
      }}
      items={[
        {
          id: "default",
          text: `Use Global Setting (${
            globalMitmEnabled ? "Enabled" : "Disabled"
          })`,
        },
        { id: "enable", text: "Enable (Inspect HTTPS)" },
        { id: "disable", text: "Disable (Pass Through)" },
      ]}
    />
    <div
      class="rd-mitm-badge"
      class:rd-mitm-on={mitmEffective}
      class:rd-mitm-off={!mitmEffective}
    >
      {mitmEffective
        ? "✓ MITM Active — full inspection on HTTP and HTTPS"
        : "✗ MITM Inactive — URL patterns, content-type, and keyword filters work on HTTP only"}
    </div>
  </div>

  <!-- Description -->
  <div class="rd-field">
    <span class="rd-field-label">Description</span>
    <TextArea
      rows={3}
      hideLabel
      bind:value={rule.description}
      placeholder="Optional description"
    />
  </div>

  <hr />

  <!-- User Matching Rules -->
  <div class="rd-section-header">
    <span class="rd-section-title">User Match Criteria</span>
  </div>

  <p class="rd-filter-hint">
    Rules apply to everyone or specific users. The rule is skipped if it does
    not match any specified users. Empty input means all users are included.
  </p>

  <!-- Users -->
  <div class="rd-field">
    <span class="rd-field-label">Users</span>
    <div class="rd-add-row">
      <div class="rd-add-input">
        <ComboBox
          size="sm"
          items={availableUsers}
          bind:selectedId={userSelectedId}
          bind:value={userInput}
          shouldFilterItem={shouldFilterUserItem}
          placeholder="Search users (empty = all users)"
          on:select={(e) => {
            if (e.detail.selectedItem) addUser(e.detail.selectedItem.text);
          }}
          on:keydown={(e) => {
            if (e.key === "Enter" && userInput) {
              e.preventDefault();
              addUser();
            }
          }}
        />
      </div>
      <Button size="small" on:click={() => addUser()}>Add</Button>
    </div>
    {#if rule.users && rule.users.length > 0}
      <div class="rd-tags">
        {#each rule.users as user}
          <Tag size="sm" filter on:close={() => removeUser(user)}>{user}</Tag>
        {/each}
      </div>
    {/if}
  </div>

  <hr />

  <!-- Domain Matching Rules -->
  <div class="rd-section-header">
    <span class="rd-section-title">Rule Selection Criteria</span>
  </div>

  <p class="rd-filter-hint">
    All conditions must match for the rule to apply. Empty conditions are
    effectively "match all." URL patterns, content-type, and keyword filters
    always work on HTTP. For HTTPS, they require MITM to be active.
  </p>

  <!-- Domain Patterns -->
  <div class="rd-field">
    <span class="rd-field-label">Domain Patterns</span>
    <div class="rd-add-row">
      <div class="rd-add-input">
        <TextInput
          size="sm"
          bind:value={domainPatternInput}
          placeholder="e.g., *.ads.com"
          on:keydown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addDomainPattern();
            }
          }}
        />
      </div>
      <Button size="small" on:click={addDomainPattern}>Add</Button>
    </div>
    {#if rule.domain_patterns && rule.domain_patterns.length > 0}
      <div class="rd-tags">
        {#each rule.domain_patterns as pattern}
          <Tag size="sm" filter on:close={() => removeDomainPattern(pattern)}
            >{pattern}</Tag
          >
        {/each}
      </div>
    {/if}
  </div>

  <!-- Domain Lists -->
  {#if availableDomainLists.length > 0}
    <div class="rd-field">
      <span class="rd-field-label">Domain Lists</span>
      <MultiSelect
        size="sm"
        titleText=""
        label="Select domain lists..."
        items={availableDomainLists}
        selectedIds={rule.domain_lists || []}
        on:select={(e) => {
          rule.domain_lists = e.detail.selectedIds;
        }}
      />
    </div>
  {/if}

  <!-- URL Patterns -->
  <div class="rd-field">
    <span class="rd-field-label">
      URL Patterns
      {#if !mitmEffective}<span class="rd-info-badge">HTTPS requires MITM</span
        >{/if}
    </span>
    <p class="rd-field-hint">
      Regex patterns matched against the full URL. If any pattern matches, the
      rule applies. If none match, the rule is skipped for this request.
      {#if !mitmEffective}Only evaluated on HTTP traffic when MITM is inactive.{/if}
    </p>
    <div class="rd-add-row">
      <div class="rd-add-input">
        <TextInput
          size="sm"
          bind:value={urlRegexInput}
          placeholder="e.g., /ads/.* or /tracker"
          on:keydown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addUrlRegex();
            }
          }}
        />
      </div>
      <Button size="small" on:click={addUrlRegex}>Add</Button>
    </div>
    {#if rule.url_regex_patterns && rule.url_regex_patterns.length > 0}
      <div class="rd-tags">
        {#each rule.url_regex_patterns as pattern}
          <Tag size="sm" filter on:close={() => removeUrlRegex(pattern)}
            >{pattern}</Tag
          >
        {/each}
      </div>
    {/if}
  </div>

  <!-- Content Type -->
  <div class="rd-field">
    <span class="rd-field-label">
      Content Type
      {#if !mitmEffective}<span class="rd-info-badge">HTTPS requires MITM</span
        >{/if}
    </span>
    <p class="rd-field-hint">
      Match by response MIME type. If specified and the response content-type
      doesn't match, the rule is skipped.
      {#if !mitmEffective}Only evaluated on HTTP traffic when MITM is inactive.{/if}
    </p>
    <div class="rd-add-row">
      <div class="rd-add-input">
        <ComboBox
          size="sm"
          items={MIME_TYPE_ITEMS}
          bind:selectedId={contentTypeSelectedId}
          bind:value={contentTypeInput}
          shouldFilterItem={shouldFilterMimeItem}
          placeholder="Search or type a MIME type"
          on:select={(e) => {
            if (e.detail.selectedItem)
              addContentType(e.detail.selectedItem.text);
          }}
          on:keydown={(e) => {
            if (e.key === "Enter" && contentTypeInput) {
              e.preventDefault();
              addContentType();
            }
          }}
        />
      </div>
      <Button size="small" on:click={() => addContentType()}>Add</Button>
    </div>
    {#if rule.blocked_content_types && rule.blocked_content_types.length > 0}
      <div class="rd-tags">
        {#each rule.blocked_content_types as type}
          <Tag size="sm" filter on:close={() => removeContentType(type)}
            >{type}</Tag
          >
        {/each}
      </div>
    {/if}
  </div>

  <hr />

  <!-- Matching Results -->
  <div class="rd-section-header">
    <span class="rd-section-title">Matching Results</span>
  </div>

  <p class="rd-filter-hint">
    When all match criteria above are satisfied, these results are applied.
    Keywords can force a block even if the action is "Allow."
  </p>

  <!-- Keyword Filter Toggle -->
  <div class="rd-field">
    <div class="rd-toggle-card">
      <div class="rd-toggle-info">
        <span class="rd-field-label" style="margin-bottom:0">
          Keyword Filter
          {#if !mitmEffective}<span class="rd-info-badge"
              >HTTPS requires MITM</span
            >{/if}
        </span>
        <span class="rd-toggle-desc">
          {#if !mitmEffective}
            Keywords are scanned on HTTP traffic only when MITM is inactive.
          {:else if rule.keyword_filter_enabled}
            Page text will be scanned for blocked keywords. If the watermark
            score is reached, the page is <strong>force-blocked</strong> regardless
            of the rule action below.
          {:else}
            Keyword scanning is disabled for this rule.
          {/if}
        </span>
      </div>
      <Toggle
        size="sm"
        bind:toggled={rule.keyword_filter_enabled}
        hideLabel
        labelA=""
        labelB=""
      />
    </div>
  </div>

  <!-- Action -->
  <div class="rd-field">
    <span class="rd-field-label">Rule Action</span>
    <Dropdown
      size="sm"
      titleText=""
      selectedId={rule.action}
      on:select={(e) => {
        rule.action = e.detail.selectedId;
      }}
      items={[
        { id: "allow", text: "Allow" },
        { id: "block", text: "Block" },
      ]}
    />
    <p class="rd-field-hint">
      {#if rule.action === "block"}
        Matching requests will be blocked and shown a block page.
      {:else}
        Matching requests will be allowed through the proxy.
      {/if}
    </p>
  </div>

  <!-- Actions -->
  <div class="rd-actions">
    <Button
      size="small"
      kind="primary"
      icon={Save}
      disabled={saving}
      on:click={saveRule}
    >
      {saving ? "Saving..." : "Save Rule"}
    </Button>
    {#if !isNew && rule.id}
      <Button
        size="small"
        kind="danger-tertiary"
        icon={TrashCan}
        disabled={deleting}
        on:click={deleteRule}
      >
        {deleting ? "Deleting..." : "Delete Rule"}
      </Button>
    {/if}
  </div>

  {#if saving || deleting}
    <div style="margin-top: 8px;">
      <InlineLoading description={saving ? "Saving..." : "Deleting..."} />
    </div>
  {/if}

  <hr />

  <!-- Rule Tester -->
  <div class="rd-section-header">
    <span class="rd-section-title">Rule Tester</span>
  </div>

  <p class="rd-filter-hint">
    Test a URL against this rule's current configuration (no save required).
    Enable <strong>Live Test</strong> to fetch the actual page and evaluate content-type
    and keyword filters.
  </p>

  <div class="rd-field">
    <span class="rd-field-label">Test URL</span>
    <div class="rd-add-row">
      <div class="rd-add-input">
        <TextInput
          size="sm"
          bind:value={testUrl}
          placeholder="e.g., https://example.com/path/page"
          on:keydown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              testRule();
            }
          }}
        />
      </div>
      <Button
        size="small"
        kind="secondary"
        on:click={testRule}
        disabled={testLoading}
      >
        {testLoading ? "Testing…" : "Test"}
      </Button>
    </div>

    <div class="rt-options">
      {#if rule.users && rule.users.length > 0}
        <div class="rt-option">
          <TextInput
            size="sm"
            bind:value={testUser}
            placeholder="Test as user (optional)"
            labelText="User"
          />
        </div>
      {/if}
    </div>

    <div class="rt-live-row">
      <span class="rt-live-hint">
        Fetches URL from the server to test content-type & keywords
      </span>
    </div>

    <div class="rt-live-toggle">
      <Toggle
        size="sm"
        labelText="Live Test"
        labelA="Off"
        labelB="On"
        bind:toggled={testLive}
      />
    </div>

    {#if testLoading}
      <div style="margin-top: 8px;">
        <InlineLoading
          description={testLive
            ? "Fetching URL and evaluating…"
            : "Evaluating rule…"}
        />
      </div>
    {/if}

    {#if testResult}
      {#if testResult.error}
        <div class="rt-result rt-error">{testResult.error}</div>
      {:else}
        <div
          class="rt-result"
          class:rt-block={testResult.outcome === "block"}
          class:rt-allow={testResult.outcome === "allow"}
          class:rt-skip={testResult.outcome === "skip" ||
            testResult.outcome === "error"}
        >
          <div class="rt-outcome">{testResult.reason}</div>

          {#if testResult.response_status}
            <div class="rt-live-info">
              <span class="rt-live-badge"
                >HTTP {testResult.response_status}</span
              >
              {#if testResult.response_content_type}
                <span class="rt-live-badge"
                  >{testResult.response_content_type}</span
                >
              {/if}
              {#if testResult.keyword_score > 0 || testResult.keyword_watermark > 0}
                <span
                  class="rt-live-badge"
                  class:rt-live-badge-warn={testResult.keyword_score >
                    testResult.keyword_watermark}
                >
                  Keywords: {testResult.keyword_score} / {testResult.keyword_watermark}
                </span>
              {/if}
            </div>
          {/if}

          <div class="rt-steps">
            {#each testResult.steps as s}
              <div class="rt-step">
                <span class="rt-step-num">{s.step}</span>
                <span class="rt-step-name">{s.name}</span>
                <span
                  class="rt-step-icon"
                  class:rt-pass={s.result === "pass" || s.result === "allow"}
                  class:rt-fail={s.result === "fail" || s.result === "skip"}
                  class:rt-info={s.result === "info"}
                  class:rt-block-icon={s.result === "block"}
                >
                  {s.result === "pass" || s.result === "allow"
                    ? "✓"
                    : s.result === "fail" || s.result === "skip"
                    ? "✗"
                    : s.result === "block"
                    ? "⊘"
                    : "ℹ"}
                </span>
                <span class="rt-step-detail">{s.detail}</span>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    {/if}
  </div>
</div>

<style>
  .rd-header {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 16px;
  }

  .rd-back {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    cursor: pointer;
    color: #525252;
    flex-shrink: 0;
    transition: background-color 0.12s;
  }
  .rd-back:hover {
    background: #e0e0e0;
  }
  .rd-back:active {
    background: #c6c6c6;
  }

  .rd-header-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .rd-header-label {
    font-size: 0.85rem;
    font-weight: 600;
    color: #161616;
    letter-spacing: 0.02em;
  }

  .rd-scroll {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding-bottom: 40px; /* room for mobile keyboards */
  }

  .rd-toggle-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }
  .rd-toggle-card :global(.bx--form-item) {
    flex: 0 0 auto;
  }

  .rd-toggle-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .rd-toggle-desc {
    font-size: 0.75rem;
    color: #6f6f6f;
  }

  .rd-section-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 4px 0 6px 0;
  }

  .rd-section-title {
    font-size: 0.75rem;
    font-weight: 600;
    color: #525252;
    letter-spacing: 0.02em;
    text-transform: uppercase;
  }

  .rd-time-inputs {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .rd-time {
    font-size: 0.875rem;
    font-family: inherit;
    padding: 4px 8px;
    border: 1px solid #8d8d8d;
    border-radius: 0;
    background: #f4f4f4;
    color: #161616;
    height: 32px;
    min-width: 110px;
    outline: none;
  }
  .rd-time:focus {
    border-color: #0f62fe;
    outline: 2px solid #0f62fe;
    outline-offset: -2px;
  }

  .rd-time-sep {
    font-size: 0.85rem;
    color: #525252;
  }

  .rd-val-error {
    font-size: 0.8rem;
    color: #da1e28;
    margin-top: 6px;
    font-weight: 500;
  }

  .rd-field {
    background: #fff;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    padding: 14px 16px;
    margin-bottom: 8px;
  }

  .rd-field-label {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
    color: #525252;
    margin-bottom: 6px;
    letter-spacing: 0.02em;
  }

  .rd-add-row {
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }

  .rd-add-input {
    flex: 1;
    min-width: 0;
  }

  .rd-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 8px;
  }

  .rd-actions {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-top: 8px;
    padding: 14px 0;
  }

  .rd-filter-hint {
    font-size: 0.8rem;
    color: #6f6f6f;
    margin: 4px 0 8px 0;
    font-style: italic;
  }

  .rd-field-hint {
    font-size: 0.75rem;
    color: #6f6f6f;
    margin: 4px 0 6px 0;
  }

  .rd-info-badge {
    display: inline-block;
    font-size: 0.65rem;
    font-weight: 600;
    color: #0043ce;
    background: #edf5ff;
    border: 1px solid #0043ce;
    border-radius: 3px;
    padding: 1px 6px;
    margin-left: 6px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    vertical-align: middle;
  }

  .rd-mitm-badge {
    font-size: 0.75rem;
    font-weight: 500;
    margin-top: 6px;
    padding: 4px 8px;
    border-radius: 4px;
  }
  .rd-mitm-on {
    color: #198038;
    background: #defbe6;
  }
  .rd-mitm-off {
    color: #da1e28;
    background: #fff1f1;
  }

  @media (max-width: 671px) {
    .rd-field {
      padding: 12px 12px;
    }

    .rd-add-row {
      flex-direction: column;
      align-items: center;
    }
    .rd-add-row .rd-add-input {
      width: 100%;
    }
    .rd-add-row :global(.bx--btn) {
      max-width: 280px;
      width: 100%;
    }

    .rd-time-inputs {
      flex-wrap: wrap;
    }

    .rd-actions {
      flex-direction: column;
      align-items: center;
    }
    .rd-actions :global(.bx--btn) {
      max-width: 280px;
      width: 100%;
    }
  }

  /* Rule Tester */
  .rt-result {
    margin-top: 12px;
    border-radius: 6px;
    padding: 12px 14px;
    font-size: 0.85rem;
  }
  .rt-error {
    background: #fff1f1;
    color: #da1e28;
    border: 1px solid #da1e28;
  }
  .rt-block {
    background: #fff1f1;
    border: 1px solid #da1e28;
  }
  .rt-allow {
    background: #defbe6;
    border: 1px solid #198038;
  }
  .rt-skip {
    background: #f4f4f4;
    border: 1px solid #8d8d8d;
  }
  .rt-outcome {
    font-weight: 600;
    font-size: 0.9rem;
    margin-bottom: 10px;
  }
  .rt-block .rt-outcome {
    color: #da1e28;
  }
  .rt-allow .rt-outcome {
    color: #198038;
  }
  .rt-skip .rt-outcome {
    color: #525252;
  }
  .rt-steps {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .rt-step {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 0.8rem;
    line-height: 1.4;
  }
  .rt-step-num {
    flex: 0 0 18px;
    text-align: center;
    font-weight: 600;
    color: #525252;
    font-size: 0.7rem;
    background: #e0e0e0;
    border-radius: 3px;
    padding: 1px 0;
    margin-top: 1px;
  }
  .rt-step-name {
    flex: 0 0 100px;
    font-weight: 600;
    color: #393939;
  }
  .rt-step-icon {
    flex: 0 0 16px;
    text-align: center;
    font-weight: 700;
    font-size: 0.85rem;
    margin-top: -1px;
  }
  .rt-pass {
    color: #198038;
  }
  .rt-fail {
    color: #da1e28;
  }
  .rt-info {
    color: #0043ce;
  }
  .rt-block-icon {
    color: #da1e28;
  }
  .rt-step-detail {
    flex: 1;
    color: #525252;
    min-width: 0;
  }

  /* Rule Tester options & live info */
  .rt-options {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    margin-top: 10px;
  }
  .rt-option {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .rt-live-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 10px;
  }
  .rt-live-toggle {
    flex-shrink: 0;
  }
  .rt-live-hint {
    font-size: 0.75rem;
    color: #6f6f6f;
  }
  .rt-live-info {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 10px;
  }
  .rt-live-badge {
    display: inline-block;
    font-size: 0.75rem;
    font-weight: 500;
    padding: 2px 8px;
    border-radius: 4px;
    background: #e0e0e0;
    color: #393939;
  }
  .rt-live-badge-warn {
    background: #fff1f1;
    color: #da1e28;
    font-weight: 600;
  }
</style>
