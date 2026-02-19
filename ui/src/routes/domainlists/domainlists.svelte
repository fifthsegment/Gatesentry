<script lang="ts">
  import {
    Button,
    ComposedModal,
    Dropdown,
    InlineLoading,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Tag,
    TextArea,
    TextInput,
  } from "carbon-components-svelte";
  import {
    AddAlt,
    Edit,
    ListBoxes,
    Renew,
    RowDelete,
    Save,
  } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import { notificationstore } from "../../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../../lib/utils";

  let lists = [];
  let loading = true;
  let showForm = false;
  let editingList = null;

  // Form fields
  let formName = "";
  let formDescription = "";
  let formCategory = "";
  let formSource = "local";
  let formUrl = "";
  let formDomains = "";

  // Sort
  let sortAsc = true;

  $: sortedLists = [...lists].sort((a, b) => {
    const cmp = (a.name || "").localeCompare(b.name || "", undefined, {
      sensitivity: "base",
    });
    return sortAsc ? cmp : -cmp;
  });

  function formatDate(dateStr) {
    if (!dateStr) return "—";
    const d = new Date(dateStr);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    const mins = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);
    if (mins < 1) return "Just now";
    if (mins < 60) return `${mins}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return d.toLocaleDateString();
  }

  function getHeaders() {
    const token = localStorage.getItem("jwt");
    return {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };
  }

  async function loadLists() {
    loading = true;
    try {
      const res = await fetch(getBasePath() + "/api/domainlists", {
        headers: getHeaders(),
      });
      if (res.ok) {
        const data = await res.json();
        lists = (data.lists || []).map((l) => ({
          ...l,
          id: l.id,
        }));
      }
    } catch (e) {
      console.error("Failed to load domain lists:", e);
    }
    loading = false;
  }

  onMount(loadLists);

  function openAddForm() {
    editingList = null;
    formName = "";
    formDescription = "";
    formCategory = "";
    formSource = "local";
    formUrl = "";
    formDomains = "";
    showForm = true;
  }

  async function openEditForm(list) {
    editingList = list;
    formName = list.name;
    formDescription = list.description || "";
    formCategory = list.category || "";
    formSource = list.source || "local";
    formUrl = list.url || "";
    formDomains = "";

    // Load existing domains for local lists
    if (list.source === "local") {
      try {
        const res = await fetch(
          getBasePath() + "/api/domainlists/" + list.id + "/domains",
          { headers: getHeaders() },
        );
        if (res.ok) {
          const data = await res.json();
          formDomains = (data.domains || []).join("\n");
        }
      } catch (e) {
        console.error("Failed to load domains for list:", e);
      }
    }

    showForm = true;
  }

  async function saveForm() {
    const body = {
      name: formName,
      description: formDescription,
      category: formCategory,
      source: formSource,
      url: formSource === "url" ? formUrl : "",
    };

    // For local lists, always include domains
    if (formSource === "local") {
      body.domains = formDomains
        .split("\n")
        .map((d) => d.trim())
        .filter((d) => d.length > 0);
    }

    try {
      let res;
      if (editingList) {
        res = await fetch(
          getBasePath() + "/api/domainlists/" + editingList.id,
          {
            method: "PUT",
            headers: getHeaders(),
            body: JSON.stringify(body),
          },
        );
      } else {
        res = await fetch(getBasePath() + "/api/domainlists", {
          method: "POST",
          headers: getHeaders(),
          body: JSON.stringify(body),
        });
      }

      if (res.ok) {
        notificationstore.add(
          createNotificationSuccess(
            {
              title: $_("Success"),
              subtitle: editingList
                ? $_("Domain list updated")
                : $_("Domain list created"),
            },
            $_,
          ),
        );
        showForm = false;
        await loadLists();
      } else {
        const errText = await res.text();
        notificationstore.add(
          createNotificationError(
            {
              title: $_("Error"),
              subtitle: errText || $_("Failed to save domain list"),
            },
            $_,
          ),
        );
      }
    } catch (e) {
      notificationstore.add(
        createNotificationError(
          {
            title: $_("Error"),
            subtitle: $_("Failed to save domain list"),
          },
          $_,
        ),
      );
    }
  }

  async function deleteList(id) {
    if (!confirm($_("Are you sure you want to delete this domain list?")))
      return;
    try {
      const res = await fetch(getBasePath() + "/api/domainlists/" + id, {
        method: "DELETE",
        headers: getHeaders(),
      });
      if (res.ok) {
        notificationstore.add(
          createNotificationSuccess(
            {
              title: $_("Success"),
              subtitle: $_("Domain list deleted"),
            },
            $_,
          ),
        );
        await loadLists();
      }
    } catch (e) {
      notificationstore.add(
        createNotificationError(
          {
            title: $_("Error"),
            subtitle: $_("Failed to delete domain list"),
          },
          $_,
        ),
      );
    }
  }

  async function refreshList(id) {
    try {
      const res = await fetch(
        getBasePath() + "/api/domainlists/" + id + "/refresh",
        {
          method: "POST",
          headers: getHeaders(),
        },
      );
      if (res.ok) {
        notificationstore.add(
          createNotificationSuccess(
            {
              title: $_("Success"),
              subtitle: $_("Domain list refresh started"),
            },
            $_,
          ),
        );
        setTimeout(loadLists, 2000);
      }
    } catch (e) {
      notificationstore.add(
        createNotificationError(
          {
            title: $_("Error"),
            subtitle: $_("Failed to refresh domain list"),
          },
          $_,
        ),
      );
    }
  }
</script>

<div class="gs-page-title">
  <ListBoxes size={24} />
  <h2>{$_("Domain Lists")}</h2>
</div>
<p class="dl-subtitle">
  {$_(
    "Domain Lists are reusable collections of domains. They can be URL-sourced (downloaded from a remote URL) or locally managed. Assign them to DNS filtering on the DNS page, or reference them in Rules for per-user enforcement.",
  )}
  <a href="https://github.com/hagezi/dns-blocklists" target="_blank"
    >{$_("Get more block lists from here")}</a
  >
</p>

<!-- Sticky top bar: Create button + sort toggle -->
<div class="dl-topbar">
  <Button size="small" icon={AddAlt} on:click={openAddForm}>
    {$_("Create Domain List")}
  </Button>
  <button class="dl-sort-btn" on:click={() => (sortAsc = !sortAsc)}>
    Name {sortAsc ? "▲" : "▼"}
  </button>
</div>

{#if showForm}
  <ComposedModal
    open
    preventCloseOnClickOutside={true}
    on:submit={saveForm}
    on:close={() => {
      showForm = false;
    }}
  >
    <ModalHeader
      title={editingList ? $_("Edit Domain List") : $_("Create Domain List")}
    />
    <ModalBody hasForm>
      <TextInput
        labelText={$_("Name")}
        type="text"
        bind:value={formName}
        placeholder="e.g., Ad Servers, Social Media"
        size="sm"
      />
      <br />
      <TextInput
        labelText={$_("Description")}
        type="text"
        bind:value={formDescription}
        placeholder="Optional description"
        size="sm"
      />
      <br />
      <TextInput
        labelText={$_("Category")}
        type="text"
        bind:value={formCategory}
        placeholder="e.g., ads, malware, social"
        size="sm"
      />
      <br />
      <Dropdown
        titleText={$_("Source Type")}
        size="sm"
        selectedId={formSource}
        disabled={editingList !== null}
        on:select={(e) => {
          formSource = e.detail.selectedId;
        }}
        items={[
          { id: "local", text: "Local (manually managed)" },
          { id: "url", text: "Remote URL (auto-downloaded)" },
        ]}
      />
      <br />
      {#if formSource === "url"}
        <TextInput
          labelText={$_("Block List URL")}
          type="text"
          bind:value={formUrl}
          placeholder="https://example.com/blocklist.txt"
          size="sm"
        />
      {/if}
      {#if formSource === "local"}
        <TextArea
          labelText={$_("Domains (one per line)")}
          bind:value={formDomains}
          placeholder="example.com&#10;ads.tracker.com&#10;malware.org"
          rows={8}
        />
      {/if}
    </ModalBody>
    <ModalFooter
      primaryButtonDisabled={!formName.trim() ||
        (formSource === "url" && !formUrl.trim())}
      primaryButtonIcon={Save}
      primaryButtonText={editingList ? $_("Update") : $_("Create")}
    />
  </ComposedModal>
{/if}

{#if loading}
  <InlineLoading description="Loading domain lists..." />
{:else if sortedLists.length === 0}
  <div class="dl-empty">
    <p>{$_("No domain lists yet.")}</p>
    <Button size="small" icon={AddAlt} on:click={openAddForm}>
      {$_("Create your first Domain List")}
    </Button>
  </div>
{:else}
  <div class="dl-grid">
    {#each sortedLists as list (list.id)}
      <div class="dl-card">
        <div class="dl-card-header">
          <div class="dl-card-title">{list.name}</div>
          <div class="dl-card-actions">
            <button
              class="dl-icon-btn"
              title={$_("Edit")}
              on:click={() => openEditForm(list)}
            >
              <Edit size={16} />
            </button>
            {#if list.source === "url"}
              <button
                class="dl-icon-btn"
                title={$_("Refresh")}
                on:click={() => refreshList(list.id)}
              >
                <Renew size={16} />
              </button>
            {/if}
            <button
              class="dl-icon-btn dl-icon-btn--danger"
              title={$_("Delete")}
              on:click={() => deleteList(list.id)}
            >
              <RowDelete size={16} />
            </button>
          </div>
        </div>
        {#if list.description}
          <div class="dl-card-desc">{list.description}</div>
        {/if}
        <div class="dl-card-meta">
          <Tag size="sm" type={list.source === "url" ? "blue" : "green"}>
            {list.source === "url" ? "URL" : "Local"}
          </Tag>
          {#if list.category}
            <Tag size="sm" type="outline">{list.category}</Tag>
          {/if}
          <span class="dl-card-stat">
            <strong>{(list.entry_count || 0).toLocaleString()}</strong> domains
          </span>
        </div>
        <div class="dl-card-footer">
          <span class="dl-card-updated"
            >Updated {formatDate(list.last_updated)}</span
          >
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .dl-subtitle {
    margin-top: 8px;
    margin-bottom: 16px;
    color: #525252;
    line-height: 1.5;
  }

  .dl-topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
    flex-wrap: wrap;
    gap: 8px;
  }

  .dl-sort-btn {
    background: none;
    border: 1px solid #e0e0e0;
    border-radius: 4px;
    padding: 6px 12px;
    font-size: 0.8125rem;
    color: #525252;
    cursor: pointer;
    white-space: nowrap;
  }
  .dl-sort-btn:hover {
    background: #e0e0e0;
  }

  .dl-empty {
    text-align: center;
    padding: 48px 16px;
    color: #525252;
  }
  .dl-empty p {
    margin-bottom: 16px;
    font-size: 1rem;
  }

  /* Card grid */
  .dl-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 12px;
  }

  .dl-card {
    background: #fff;
    border: 1px solid #e0e0e0;
    border-radius: 4px;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    transition: box-shadow 0.15s ease;
  }
  .dl-card:hover {
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  }

  .dl-card-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
  }

  .dl-card-title {
    font-size: 0.9375rem;
    font-weight: 600;
    color: #161616;
    word-break: break-word;
    line-height: 1.3;
  }

  .dl-card-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

  .dl-icon-btn {
    background: none;
    border: none;
    padding: 6px;
    cursor: pointer;
    color: #525252;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .dl-icon-btn:hover {
    background: #e0e0e0;
    color: #161616;
  }
  .dl-icon-btn--danger:hover {
    background: #fff1f1;
    color: #da1e28;
  }

  .dl-card-desc {
    font-size: 0.8125rem;
    color: #6f6f6f;
    line-height: 1.4;
  }

  .dl-card-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
  }

  .dl-card-stat {
    font-size: 0.8125rem;
    color: #393939;
    margin-left: auto;
  }

  .dl-card-footer {
    border-top: 1px solid #f4f4f4;
    padding-top: 6px;
    margin-top: 2px;
  }

  .dl-card-updated {
    font-size: 0.75rem;
    color: #a8a8a8;
  }

  /* Mobile: single column */
  @media (max-width: 671px) {
    .dl-grid {
      grid-template-columns: 1fr;
      gap: 10px;
    }
    .dl-card {
      padding: 12px;
    }
    .dl-card-stat {
      margin-left: 0;
      flex-basis: 100%;
    }
  }
</style>
