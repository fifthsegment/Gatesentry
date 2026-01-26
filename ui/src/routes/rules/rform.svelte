<script lang="ts">
  import {
    Button,
    Column,
    Dropdown,
    Row,
    Tag,
    TextArea,
    TextInput,
    Toggle,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
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
    time_restriction: null,
    users: [],
    description: "",
  };

  export let index;
  export let expanded = false;
  
  import { createEventDispatcher } from "svelte";
  import Timepicker from "../../components/timepicker.svelte";
  import { ChevronDown, ChevronUp, RowDelete } from "carbon-icons-svelte";
  const dispatch = createEventDispatcher();

  function toggleExpand() {
    dispatch("toggle");
  }

  // Ensure arrays are initialized
  $: {
    if (!rule.blocked_content_types) rule.blocked_content_types = [];
    if (!rule.url_regex_patterns) rule.url_regex_patterns = [];
    if (!rule.users) rule.users = [];
  }

  let contentTypeInput = "";
  let urlRegexInput = "";
  let userInput = "";

  function addContentType() {
    if (contentTypeInput.trim()) {
      rule.blocked_content_types = [
        ...rule.blocked_content_types,
        contentTypeInput.trim(),
      ];
      contentTypeInput = "";
    }
  }

  function removeContentType(type) {
    rule.blocked_content_types = rule.blocked_content_types.filter(
      (t) => t !== type
    );
  }

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
      (p) => p !== pattern
    );
  }

  function addUser() {
    if (userInput.trim()) {
      rule.users = [...rule.users, userInput.trim()];
      userInput = "";
    }
  }

  function removeUser(user) {
    rule.users = rule.users.filter((u) => u !== user);
  }

  $: showContentTypeOptions =
    rule.mitm_action === "enable" &&
    rule.block_type !== "none" &&
    (rule.block_type === "content_type" || rule.block_type === "both");

  $: showUrlRegexOptions =
    rule.mitm_action === "enable" &&
    rule.block_type !== "none" &&
    (rule.block_type === "url_regex" || rule.block_type === "both");
</script>

<div class="simple-border">
  <!-- Collapsed Summary View -->
  {#if !expanded}
    <div class="rule-summary" on:click={toggleExpand}>
      <div class="summary-content">
        <div class="summary-left">
          <span class="rule-number">#{index + 1}</span>
          <strong>{rule.name || `Rule ${index + 1}`}</strong>
          {#if rule.domain}
            <span class="domain-badge">{rule.domain}</span>
          {/if}
        </div>
        <div class="summary-right">
          <span class="action-badge action-{rule.action}">{rule.action}</span>
          <span class="mitm-badge mitm-{rule.mitm_action}">
            MITM: {rule.mitm_action}
          </span>
          <Button
            size="small"
            kind="ghost"
            icon={ChevronDown}
            iconDescription="Expand"
          />
        </div>
      </div>
    </div>
  {:else}
    <!-- Expanded Edit View -->
    <div class="rule-header" on:click={toggleExpand}>
      <h5>{$_("Rule")} {index + 1}</h5>
      <Button
        size="small"
        kind="ghost"
        icon={ChevronUp}
        iconDescription="Collapse"
      />
    </div>
    
    <div class="rule-form">
      <table class="rule-table">
        <tbody>
          <!-- Enabled Toggle -->
          <tr>
            <td class="label-col">{$_("Enabled")}</td>
            <td class="input-col">
              <Toggle
                size="sm"
                bind:toggled={rule.enabled}
                hideLabel
                labelA=""
                labelB=""
              />
            </td>
          </tr>

          <!-- Name -->
          <tr>
            <td class="label-col">{$_("Name")}</td>
            <td class="input-col">
              <TextInput
                size="sm"
                type="text"
                bind:value={rule.name}
                placeholder="Rule Name"
              />
            </td>
          </tr>

          <!-- Domain -->
          <tr>
            <td class="label-col">{$_("Domain")} *</td>
            <td class="input-col">
              <TextInput
                size="sm"
                type="text"
                bind:value={rule.domain}
                placeholder="*.example.com or example.com"
              />
            </td>
          </tr>

          <!-- Priority -->
          <tr>
            <td class="label-col">{$_("Priority")}</td>
            <td class="input-col">
              <TextInput
                size="sm"
                type="number"
                bind:value={rule.priority}
                placeholder="0"
              />
            </td>
          </tr>

          <!-- Action - always show -->
          <tr>
            <td class="label-col">{$_("Action")}</td>
            <td class="input-col">
              <Dropdown
                size="sm"
                selectedId={rule.action}
                on:select={(e) => {
                  rule.action = e.detail.selectedId;
                }}
                items={[
                  { id: "allow", text: "Allow" },
                  { id: "block", text: "Block" },
                ]}
              />
            </td>
          </tr>

          <!-- MITM Action -->
          <tr>
            <td class="label-col">{$_("SSL Inspection")}</td>
            <td class="input-col">
              <Dropdown
                size="sm"
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
            </td>
          </tr>

          {#if rule.mitm_action === "enable"}
            <!-- Block Type -->
            <tr>
              <td class="label-col">{$_("Content Filtering")}</td>
              <td class="input-col">
                <Dropdown
                  size="sm"
                  selectedId={rule.block_type}
                  on:select={(e) => {
                    rule.block_type = e.detail.selectedId;
                  }}
                  items={[
                    { id: "none", text: "None (Allow all content)" },
                    { id: "content_type", text: "Block by Content Type" },
                    { id: "url_regex", text: "Block by URL Pattern" },
                    { id: "both", text: "Block Both" },
                  ]}
                />
              </td>
            </tr>

            {#if showContentTypeOptions}
              <!-- Blocked Content Types -->
              <tr>
                <td class="label-col">{$_("Blocked Types")}</td>
                <td class="input-col">
                  <div class="list-input">
                    <TextInput
                      size="sm"
                      type="text"
                      bind:value={contentTypeInput}
                      placeholder="e.g., video/mp4"
                      on:keydown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          addContentType();
                        }
                      }}
                    />
                    <Button size="small" on:click={addContentType}>Add</Button>
                  </div>
                  {#if rule.blocked_content_types && rule.blocked_content_types.length > 0}
                    <div class="tags" style="margin-top: 8px;">
                      {#each rule.blocked_content_types as type}
                        <Tag size="sm" filter on:close={() => removeContentType(type)}>{type}</Tag>
                      {/each}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}

            {#if showUrlRegexOptions}
              <!-- URL Patterns -->
              <tr>
                <td class="label-col">{$_("URL Patterns")}</td>
                <td class="input-col">
                  <div class="list-input">
                    <TextInput
                      size="sm"
                      type="text"
                      bind:value={urlRegexInput}
                      placeholder="e.g., /ads/.*"
                      on:keydown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          addUrlRegex();
                        }
                      }}
                    />
                    <Button size="small" on:click={addUrlRegex}>Add</Button>
                  </div>
                  {#if rule.url_regex_patterns && rule.url_regex_patterns.length > 0}
                    <div class="tags" style="margin-top: 8px;">
                      {#each rule.url_regex_patterns as pattern}
                        <Tag size="sm" filter on:close={() => removeUrlRegex(pattern)}>{pattern}</Tag>
                      {/each}
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/if}

          <!-- Time Restriction -->
          <tr>
            <td class="label-col">{$_("Time Restriction")}</td>
            <td class="input-col">
              <div style="display: flex; align-items: center; gap: 10px;">
                <Toggle
                  size="sm"
                  toggled={rule.time_restriction !== null}
                  hideLabel
                  labelA=""
                  labelB=""
                  on:toggle={(e) => {
                    if (e.detail.toggled) {
                      rule.time_restriction = { from: "09:00", to: "17:00" };
                    } else {
                      rule.time_restriction = null;
                    }
                  }}
                />
                {#if rule.time_restriction}
                  <Timepicker bind:value={rule.time_restriction.from} label="From" />
                  <span>to</span>
                  <Timepicker bind:value={rule.time_restriction.to} label="To" />
                {/if}
              </div>
            </td>
          </tr>

          <!-- Users -->
          <tr>
            <td class="label-col">{$_("Users")}</td>
            <td class="input-col">
              <div class="list-input">
                <TextInput
                  size="sm"
                  type="text"
                  bind:value={userInput}
                  placeholder="Username (empty = all users)"
                  on:keydown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addUser();
                    }
                  }}
                />
                <Button size="small" on:click={addUser}>Add</Button>
              </div>
              {#if rule.users && rule.users.length > 0}
                <div class="tags" style="margin-top: 8px;">
                  {#each rule.users as user}
                    <Tag size="sm" filter on:close={() => removeUser(user)}>{user}</Tag>
                  {/each}
                </div>
              {/if}
            </td>
          </tr>

          <!-- Description -->
          <tr>
            <td class="label-col">{$_("Description")}</td>
            <td class="input-col">
              <TextArea
                rows={2}
                bind:value={rule.description}
                placeholder="Optional description"
              />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="rule-footer">
      <Button
        size="small"
        icon={RowDelete}
        kind="danger-tertiary"
        on:click={() => dispatch("remove", index)}
      >
        Remove Rule
      </Button>
      <Button
        size="small"
        kind="primary"
        on:click={() => dispatch("save", index)}
      >
        Save Rule
      </Button>
    </div>
  {/if}
</div>

<style>
  .rule-summary {
    padding: 15px;
    cursor: pointer;
    display: flex;
    align-items: center;
    transition: background-color 0.2s;
  }

  .rule-summary:hover {
    background-color: #f4f4f4;
  }

  .summary-content {
    display: flex;
    width: 100%;
    justify-content: space-between;
    align-items: center;
  }

  .summary-left {
    display: flex;
    align-items: center;
    gap: 15px;
  }

  .summary-right {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .rule-number {
    color: #525252;
    font-weight: 500;
  }

  .domain-badge {
    background-color: #e0e0e0;
    padding: 4px 12px;
    border-radius: 12px;
    font-size: 0.875rem;
    color: #161616;
  }

  .action-badge {
    padding: 4px 12px;
    border-radius: 12px;
    font-size: 0.875rem;
    font-weight: 500;
    text-transform: uppercase;
  }

  .action-allow {
    background-color: #d0e2ff;
    color: #0043ce;
  }

  .action-block {
    background-color: #ffd7d9;
    color: #a2191f;
  }

  .mitm-badge {
    padding: 4px 12px;
    border-radius: 12px;
    font-size: 0.875rem;
    background-color: #e8daff;
    color: #6929c4;
  }

  .rule-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 15px;
    cursor: pointer;
    border-bottom: 1px solid #e0e0e0;
    background-color: #f4f4f4;
  }

  .rule-header:hover {
    background-color: #e8e8e8;
  }

  .rule-form {
    padding: 15px;
  }

  .rule-table {
    width: 100%;
    border-collapse: collapse;
  }

  .rule-table td {
    padding: 8px 10px;
    vertical-align: top;
  }

  .label-col {
    width: 150px;
    font-weight: 500;
    color: #161616;
    padding-top: 12px;
  }

  .input-col {
    padding-left: 20px;
  }

  .list-input {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }

  .rule-footer {
    display: flex;
    justify-content: space-between;
    padding: 10px 15px;
    border-top: 1px solid #e0e0e0;
    background-color: #f4f4f4;
  }

  .simple-border {
    border: 1px solid #e0e0e0;
    margin-bottom: 10px;
    border-radius: 4px;
    background-color: white;
  }
</style>
