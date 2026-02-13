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
    block_type: "none",
    blocked_content_types: [],
    url_regex_patterns: [],
    domain_patterns: [],
    domain_lists: [],
    content_domain_lists: [],
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

  $: showContentTypeOptions =
    rule.mitm_action === "enable" &&
    rule.block_type !== "none" &&
    (rule.block_type === "content_type" ||
      rule.block_type === "both" ||
      rule.block_type === "all");

  $: showUrlRegexOptions =
    rule.mitm_action === "enable" &&
    rule.block_type !== "none" &&
    (rule.block_type === "url_regex" ||
      rule.block_type === "both" ||
      rule.block_type === "all");

  $: showDomainListContentOptions =
    rule.mitm_action === "enable" &&
    rule.block_type !== "none" &&
    (rule.block_type === "domain_list" || rule.block_type === "all");

  onMount(async () => {
    try {
      const token = localStorage.getItem("jwt");
      const [listsRes, usersRes] = await Promise.all([
        fetch(getBasePath() + "/api/domainlists", {
          headers: { Authorization: `Bearer ${token}` },
        }),
        fetch(getBasePath() + "/api/users", {
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
</script>

<!-- Header bar -->
<div class="rd-header">
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <span class="rd-back" on:click={goBack} title="Back to rules">
    <ArrowLeft size={20} />
  </span>
  <div class="rd-header-text">
    <span class="rd-header-label">{isNew ? "New Rule" : "Edit Rule"}</span>
    {#if !isNew}
      <h3 class="rd-title">{rule.name || "Unnamed Rule"}</h3>
    {/if}
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

<div class="rd-scroll">
  <!-- Rule Status -->
  <div class="rd-field">
    <div class="rd-toggle-card">
      <div class="rd-toggle-info">
        <span class="rd-toggle-title">Rule Status</span>
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

  <!-- Priority -->
  <div class="rd-field">
    <span class="rd-field-label">Priority</span>
    <TextInput
      size="sm"
      hideLabel
      type="number"
      bind:value={rule.priority}
      placeholder="0"
    />
  </div>

  <!-- Action -->
  <div class="rd-field">
    <span class="rd-field-label">Action</span>
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
  </div>

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

  <!-- SSL Inspection -->
  <div class="rd-field">
    <span class="rd-field-label">SSL Inspection</span>
    <Dropdown
      size="sm"
      titleText=""
      selectedId={rule.mitm_action}
      on:select={(e) => {
        rule.mitm_action = e.detail.selectedId;
      }}
      items={[
        { id: "default", text: "Use Global Setting" },
        { id: "enable", text: "Enable (Inspect HTTPS)" },
        { id: "disable", text: "Disable (Pass Through)" },
      ]}
    />
  </div>

  {#if rule.mitm_action === "enable"}
    <!-- Content Filtering -->
    <div class="rd-field">
      <span class="rd-field-label">Content Filtering</span>
      <Dropdown
        size="sm"
        titleText=""
        selectedId={rule.block_type}
        on:select={(e) => {
          rule.block_type = e.detail.selectedId;
        }}
        items={[
          { id: "none", text: "None (Allow all content)" },
          { id: "content_type", text: "Block by Content Type" },
          { id: "url_regex", text: "Block by URL Pattern" },
          { id: "both", text: "Block Content Type + URL Pattern" },
          { id: "domain_list", text: "Block by Domain List" },
          { id: "all", text: "Block All (Type + URL + Domain List)" },
        ]}
      />
    </div>

    {#if showContentTypeOptions}
      <div class="rd-field">
        <span class="rd-field-label">Blocked Content Types</span>
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
    {/if}

    {#if showUrlRegexOptions}
      <div class="rd-field">
        <span class="rd-field-label">URL Patterns</span>
        <div class="rd-add-row">
          <div class="rd-add-input">
            <TextInput
              size="sm"
              bind:value={urlRegexInput}
              placeholder="e.g., /ads/.*"
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
    {/if}

    {#if showDomainListContentOptions && availableDomainLists.length > 0}
      <div class="rd-field">
        <span class="rd-field-label">Block Embedded Resources</span>
        <MultiSelect
          size="sm"
          titleText=""
          label="Block embedded resources from these lists..."
          items={availableDomainLists}
          selectedIds={rule.content_domain_lists || []}
          on:select={(e) => {
            rule.content_domain_lists = e.detail.selectedIds;
          }}
        />
      </div>
    {/if}
  {/if}

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
    font-size: 0.75rem;
    font-weight: 500;
    color: #6f6f6f;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .rd-title {
    font-size: 1.125rem;
    font-weight: 600;
    color: #161616;
    word-break: break-word;
    margin: 0;
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

  .rd-toggle-title {
    font-size: 0.875rem;
    font-weight: 600;
    color: #161616;
  }

  .rd-toggle-desc {
    font-size: 0.75rem;
    color: #6f6f6f;
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

  @media (max-width: 671px) {
    .rd-title {
      font-size: 1rem;
    }

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
</style>
