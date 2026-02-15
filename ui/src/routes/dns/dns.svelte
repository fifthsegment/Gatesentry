<script lang="ts">
  import { isIPv4 } from "is-ip";
  import ToggleComponent from "../../components/toggle.svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import {
    Button,
    ComposedModal,
    InlineLoading,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Tag,
    TextInput,
  } from "carbon-components-svelte";
  import {
    AddAlt,
    Edit,
    Restart,
    RowDelete,
    Save,
    ServerDns,
  } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { getBasePath } from "../../lib/navigate";
  import { notificationstore } from "../../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../../lib/utils";

  // ── Tab state ──
  let activeTab = "filters"; // "filters" | "server"

  // ── DNS Info ──
  let dnsInfo = null;
  const loadDnsInfo = async () => {
    dnsInfo = null;
    dnsInfo = await $store.api.doCall("/dns/info");
  };

  const getHumanTime = (time) => {
    if (!time) return "—";
    const date = new Date(time * 1000);
    const now = new Date();
    const diff = now.getTime() - date.getTime();
    const mins = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);
    if (mins < 1) return "Just now";
    if (mins < 60) return `${mins}m ago`;
    if (hours < 24) return `${hours}h ago`;
    if (days < 7) return `${days}d ago`;
    return date.toLocaleDateString();
  };

  // ── Domain Lists (Filters tab) ──
  let allLists = [];
  let blockListIds: string[] = [];
  let allowListIds: string[] = [];
  let listsLoaded = false;
  let showPicker = false;
  let pickerMode: "block" | "allow" = "block";

  function getHeaders() {
    const token = localStorage.getItem("jwt");
    return {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    };
  }

  async function loadAllLists() {
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

  async function loadAssignedIds() {
    try {
      const blockJson = await $store.api.getSetting("dns_domain_lists");
      if (blockJson && blockJson.Value) {
        blockListIds = JSON.parse(blockJson.Value);
      } else {
        blockListIds = [];
      }
    } catch {
      blockListIds = [];
    }
    try {
      const allowJson = await $store.api.getSetting(
        "dns_whitelist_domain_lists",
      );
      if (allowJson && allowJson.Value) {
        allowListIds = JSON.parse(allowJson.Value);
      } else {
        allowListIds = [];
      }
    } catch {
      allowListIds = [];
    }
  }

  function findList(id: string) {
    return allLists.find((l) => l.id === id);
  }

  function buildDisplayList(ids: string[]) {
    return ids.map((id) => {
      const list = findList(id);
      return {
        id,
        name: list ? list.name : id,
        source: list ? list.source : "—",
        category: list ? list.category || "" : "",
        entry_count: list ? list.entry_count || 0 : 0,
      };
    });
  }

  function getAvailableForPicker() {
    const assignedIds = pickerMode === "block" ? blockListIds : allowListIds;
    return allLists.filter((l) => !assignedIds.includes(l.id));
  }

  async function saveBlockListIds() {
    try {
      await $store.api.setSetting(
        "dns_domain_lists",
        JSON.stringify(blockListIds),
      );
      loadDnsInfo();
      notificationstore.add(
        createNotificationSuccess(
          { title: $_("Success"), subtitle: $_("Block list updated") },
          $_,
        ),
      );
    } catch {
      notificationstore.add(
        createNotificationError(
          { title: $_("Error"), subtitle: $_("Unable to save block list") },
          $_,
        ),
      );
    }
  }

  async function saveAllowListIds() {
    try {
      await $store.api.setSetting(
        "dns_whitelist_domain_lists",
        JSON.stringify(allowListIds),
      );
      loadDnsInfo();
      notificationstore.add(
        createNotificationSuccess(
          { title: $_("Success"), subtitle: $_("Allow list updated") },
          $_,
        ),
      );
    } catch {
      notificationstore.add(
        createNotificationError(
          { title: $_("Error"), subtitle: $_("Unable to save allow list") },
          $_,
        ),
      );
    }
  }

  function openPicker(mode: "block" | "allow") {
    pickerMode = mode;
    showPicker = true;
  }

  async function addListToPicker(listId: string) {
    if (pickerMode === "block") {
      blockListIds = [...blockListIds, listId];
      await saveBlockListIds();
    } else {
      allowListIds = [...allowListIds, listId];
      await saveAllowListIds();
    }
    showPicker = false;
  }

  async function removeBlockList(id: string) {
    blockListIds = blockListIds.filter((i) => i !== id);
    await saveBlockListIds();
  }

  async function removeAllowList(id: string) {
    allowListIds = allowListIds.filter((i) => i !== id);
    await saveAllowListIds();
  }

  // ── Custom A Records (Server tab) ──
  let aRecords = null;
  let domainText = "";
  let ipText = "";
  let editingRowId = null;
  let showARecordForm = false;

  const loadARecords = () => {
    aRecords = [];
    $store.api.doCall("/dns/custom_entries").then((json) => {
      aRecords = json.data.map((item, index) => ({
        ...item,
        id: index + 1,
      }));
    });
  };

  const saveARecords = () => {
    const filteredData = aRecords.map((item) => ({
      domain: item.domain,
      ip: item.ip,
    }));
    $store.api
      .doCall("/dns/custom_entries", "post", filteredData, {
        "Content-Type": "application/json",
      })
      .then((json) => {
        if (json.ok === true) {
          loadARecords();
          domainText = "";
          ipText = "";
          showARecordForm = false;
          editingRowId = null;
          loadDnsInfo();
          notificationstore.add({
            kind: "success",
            title: $_("Success:"),
            subtitle: $_("A record saved"),
          });
        } else if ("error" in json) {
          notificationstore.add(
            createNotificationError(
              { title: $_("Error"), subtitle: json.error },
              $_,
            ),
          );
          loadARecords();
        }
      })
      .catch((err) => {
        notificationstore.add({
          kind: "error",
          title: $_("Error:"),
          subtitle: $_("Unable to save: ") + err.message,
          timeout: 30000,
        });
        loadARecords();
      });
  };

  const addOrSaveARecord = () => {
    if (editingRowId) {
      aRecords[editingRowId - 1].domain = domainText;
      aRecords[editingRowId - 1].ip = ipText;
      editingRowId = null;
    } else {
      aRecords = [
        ...aRecords,
        { id: aRecords.length + 1, domain: domainText, ip: ipText },
      ];
    }
    saveARecords();
  };

  const editARecord = (rowId) => {
    editingRowId = rowId;
    const row = aRecords.find((r) => r.id === rowId);
    domainText = row.domain;
    ipText = row.ip;
    showARecordForm = true;
  };

  const deleteARecord = (rowId) => {
    aRecords = aRecords.filter((r) => r.id !== rowId);
    saveARecords();
  };

  // ── Init ──
  onMount(async () => {
    await Promise.all([loadDnsInfo(), loadAllLists(), loadAssignedIds()]);
    listsLoaded = true;
    loadARecords();
  });
</script>

<div class="gs-page-title">
  <ServerDns size={24} />
  <h2>DNS</h2>
</div>

<!-- Tab bar -->
<div class="gs-tabs">
  <button
    class="gs-tab"
    class:gs-tab--active={activeTab === "filters"}
    on:click={() => (activeTab = "filters")}
  >
    Filters
  </button>
  <button
    class="gs-tab"
    class:gs-tab--active={activeTab === "server"}
    on:click={() => (activeTab = "server")}
  >
    Server
  </button>
</div>

<!-- Picker Modal (shared) -->
{#if showPicker}
  <ComposedModal open on:close={() => (showPicker = false)}>
    <ModalHeader
      title={pickerMode === "block"
        ? $_("Add to Block Lists")
        : $_("Add to Allow Lists")}
    />
    <ModalBody>
      {#if getAvailableForPicker().length === 0}
        <p class="gs-empty" style="padding: 16px 0;">
          {$_(
            "No domain lists available. Create one on the Domain Lists page first.",
          )}
        </p>
      {:else}
        <div class="gs-row-list">
          {#each getAvailableForPicker() as list (list.id)}
            <div class="gs-row-item">
              <div class="dns-picker-info">
                <span class="dns-picker-name">{list.name}</span>
                <span class="dns-picker-meta">
                  <Tag
                    size="sm"
                    type={list.source === "url" ? "blue" : "green"}
                  >
                    {list.source === "url" ? "URL" : "Local"}
                  </Tag>
                  {#if list.category}
                    <Tag size="sm" type="outline">{list.category}</Tag>
                  {/if}
                  <span class="dns-entry-count"
                    >{(list.entry_count || 0).toLocaleString()} domains</span
                  >
                </span>
              </div>
              <Button
                size="small"
                kind="primary"
                icon={AddAlt}
                iconDescription={$_("Add")}
                on:click={() => addListToPicker(list.id)}
              />
            </div>
          {/each}
        </div>
      {/if}
    </ModalBody>
    <ModalFooter>
      <Button kind="secondary" on:click={() => (showPicker = false)}>
        {$_("Cancel")}
      </Button>
    </ModalFooter>
  </ComposedModal>
{/if}

<!-- A Record Form Modal -->
{#if showARecordForm}
  <ComposedModal
    open
    preventCloseOnClickOutside={true}
    on:submit={addOrSaveARecord}
    on:close={() => {
      showARecordForm = false;
      domainText = "";
      ipText = "";
      editingRowId = null;
    }}
  >
    <ModalHeader
      title={editingRowId ? $_("Edit A Record") : $_("Add A Record")}
    />
    <ModalBody hasForm>
      <TextInput
        labelText={$_("Domain")}
        type="text"
        bind:value={domainText}
        placeholder="domain.com"
        size="sm"
      />
      <br />
      <TextInput
        type="text"
        bind:value={ipText}
        placeholder="1.1.1.1"
        size="sm"
        labelText={$_("IP Address")}
      />
    </ModalBody>
    <ModalFooter
      primaryButtonDisabled={!isIPv4(ipText)}
      primaryButtonIcon={Save}
      primaryButtonText={$_("Save")}
    />
  </ComposedModal>
{/if}

<!-- ═══════ TAB 1: FILTERS ═══════ -->
{#if activeTab === "filters"}
  <!-- DNS Filtering toggle -->
  <div class="gs-section">
    <div class="gs-card">
      <ToggleComponent
        settingName="enable_dns_filtering"
        label="DNS Filtering"
        labelA="Disabled"
        labelB="Enabled"
        size="sm"
      />
      <p class="dns-toggle-hint">
        When filtering is disabled, the DNS server still runs and resolves all
        queries normally. You can still use Proxy Filter rules to block domains.
      </p>
    </div>
  </div>

  <!-- Domain totals -->
  <div class="gs-section">
    <div class="gs-card">
      <div class="dns-list-header">
        <div>
          <h5>Domain Totals</h5>
          <p class="dns-list-hint">
            Summary of loaded blocklist domains and update schedule.
          </p>
        </div>
      </div>
      {#if dnsInfo == null}
        <InlineLoading description="Loading..." />
      {:else}
        <div class="dns-stats-row">
          <div class="dns-stat">
            <span class="dns-stat-val"
              >{(dnsInfo?.number_domains_blocked || 0).toLocaleString()}</span
            >
            <span class="dns-stat-lbl">blocked</span>
          </div>
          <span class="dns-stat-sep"></span>
          <div class="dns-stat">
            <span class="dns-stat-val"
              >{getHumanTime(dnsInfo?.last_updated)}</span
            >
            <span class="dns-stat-lbl">updated</span>
          </div>
          <span class="dns-stat-sep"></span>
          <div class="dns-stat">
            <span class="dns-stat-val"
              >{getHumanTime(dnsInfo?.next_update)}</span
            >
            <span class="dns-stat-lbl">next</span>
          </div>
          <button
            class="dns-refresh-link"
            title="Refresh"
            on:click={loadDnsInfo}
          >
            <Restart size={16} />
          </button>
        </div>
      {/if}
    </div>
  </div>

  <!-- Allow Lists -->
  {#if !listsLoaded}
    <InlineLoading description="Loading lists..." />
  {:else}
    <div class="gs-section">
      <div class="gs-card">
        <div class="dns-list-header">
          <div>
            <h5>Allow Lists</h5>
            <p class="dns-list-hint">
              Domains in these lists are never blocked, even if they appear in a
              block list.
            </p>
          </div>
          <Button
            size="small"
            kind="tertiary"
            icon={AddAlt}
            on:click={() => openPicker("allow")}
          >
            Add
          </Button>
        </div>
        {#if allowListIds.length === 0}
          <p class="gs-empty">No allow lists assigned.</p>
        {:else}
          <div class="gs-row-list">
            {#each buildDisplayList(allowListIds) as item (item.id)}
              <div class="gs-row-item">
                <div class="dns-row-info">
                  <span class="dns-row-name">{item.name}</span>
                  <span class="dns-row-meta">
                    <Tag
                      size="sm"
                      type={item.source === "url" ? "blue" : "green"}
                    >
                      {item.source === "url" ? "URL" : "Local"}
                    </Tag>
                    {#if item.category}
                      <Tag size="sm" type="outline">{item.category}</Tag>
                    {/if}
                    <span class="dns-entry-count"
                      >{item.entry_count.toLocaleString()} domains</span
                    >
                  </span>
                </div>
                <button
                  class="dns-row-delete"
                  title={$_("Remove")}
                  on:click={() => removeAllowList(item.id)}
                >
                  <RowDelete size={16} />
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <!-- Block Lists -->
    <div class="gs-section">
      <div class="gs-card">
        <div class="dns-list-header">
          <div>
            <h5>Block Lists</h5>
            <p class="dns-list-hint">
              Domains in these lists are blocked at the DNS level for all users.
            </p>
          </div>
          <Button
            size="small"
            kind="tertiary"
            icon={AddAlt}
            on:click={() => openPicker("block")}
          >
            Add
          </Button>
        </div>
        {#if blockListIds.length === 0}
          <p class="gs-empty">No block lists assigned.</p>
        {:else}
          <div class="gs-row-list">
            {#each buildDisplayList(blockListIds) as item (item.id)}
              <div class="gs-row-item">
                <div class="dns-row-info">
                  <span class="dns-row-name">{item.name}</span>
                  <span class="dns-row-meta">
                    <Tag
                      size="sm"
                      type={item.source === "url" ? "blue" : "green"}
                    >
                      {item.source === "url" ? "URL" : "Local"}
                    </Tag>
                    {#if item.category}
                      <Tag size="sm" type="outline">{item.category}</Tag>
                    {/if}
                    <span class="dns-entry-count"
                      >{item.entry_count.toLocaleString()} domains</span
                    >
                  </span>
                </div>
                <button
                  class="dns-row-delete"
                  title={$_("Remove")}
                  on:click={() => removeBlockList(item.id)}
                >
                  <RowDelete size={16} />
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>
  {/if}
{/if}

<!-- ═══════ TAB 2: SERVER ═══════ -->
{#if activeTab === "server"}
  <!-- DNS Server toggle -->
  <div class="gs-section">
    <div class="gs-card">
      <ToggleComponent
        settingName="enable_dns_server"
        label="DNS Server"
        labelA="Not Running"
        labelB="Running"
        size="sm"
      />
      <p class="dns-toggle-hint">
        Starts or stops the GateSentry DNS server completely. When stopped,
        devices using GateSentry as their DNS server will lose DNS resolution.
      </p>
    </div>
  </div>

  <!-- Upstream resolver -->
  <div class="gs-section">
    <div class="gs-card">
      <h5>Upstream DNS Resolver</h5>
      <p class="dns-list-hint">
        The upstream DNS server used to resolve queries that are not blocked or
        overridden by custom A records.
      </p>
      <div style="margin-top: 8px; max-width: 400px;">
        <ConnectedSettingInput
          keyName="dns_resolver"
          title={$_("DNS Resolver")}
          labelText={$_("DNS Resolver")}
          type="text"
          helperText=""
        />
      </div>
    </div>
  </div>

  <!-- Custom A Records -->
  <div class="gs-section">
    <div class="gs-card">
      <div class="dns-list-header">
        <div>
          <h5>Custom A Records</h5>
          <p class="dns-list-hint">
            Domains listed here resolve to the specified IP address instead of
            querying upstream DNS.
          </p>
        </div>
        <Button
          size="small"
          kind="tertiary"
          icon={AddAlt}
          on:click={() => {
            editingRowId = null;
            domainText = "";
            ipText = "";
            showARecordForm = true;
          }}
        >
          Add
        </Button>
      </div>

      {#if aRecords == null}
        <InlineLoading description="Loading A records..." />
      {:else if aRecords.length === 0}
        <p class="gs-empty">No custom A records.</p>
      {:else}
        <div class="gs-row-list">
          {#each aRecords.sort((a, b) => b.id - a.id) as record (record.id)}
            <div class="gs-row-item">
              <div class="dns-row-info">
                <span class="dns-row-name">{record.domain}</span>
                <span class="dns-row-meta">
                  <code class="dns-ip">{record.ip}</code>
                </span>
              </div>
              <div class="dns-row-actions">
                <button
                  class="dns-row-edit"
                  title={$_("Edit")}
                  on:click={() => editARecord(record.id)}
                >
                  <Edit size={16} />
                </button>
                <button
                  class="dns-row-delete"
                  title={$_("Delete")}
                  on:click={() => deleteARecord(record.id)}
                >
                  <RowDelete size={16} />
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* ── Toggle hint ── */
  .dns-toggle-hint {
    font-size: 0.8125rem;
    color: #6f6f6f;
    line-height: 1.5;
    margin-top: 8px;
  }

  /* ── Stats row ── */
  .dns-stats-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
  }
  .dns-stat {
    display: flex;
    align-items: baseline;
    gap: 4px;
  }
  .dns-stat-val {
    font-size: 0.9375rem;
    font-weight: 700;
    color: #161616;
  }
  .dns-stat-lbl {
    font-size: 0.75rem;
    color: #6f6f6f;
  }
  .dns-stat-sep {
    width: 1px;
    height: 14px;
    background: #d0d0d0;
    flex-shrink: 0;
  }
  .dns-refresh-link {
    background: none;
    border: none;
    padding: 4px;
    cursor: pointer;
    color: #6f6f6f;
    border-radius: 4px;
    display: flex;
    align-items: center;
  }
  .dns-refresh-link:hover {
    color: #0f62fe;
  }

  /* ── List headers ── */
  .dns-list-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 8px;
  }
  .dns-list-header > div:first-child {
    flex: 1;
    min-width: 0;
  }
  .dns-list-hint {
    font-size: 0.8125rem;
    color: #6f6f6f;
    line-height: 1.4;
    margin-top: 2px;
  }

  /* ── Row content ── */
  .dns-row-info {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .dns-row-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: #161616;
    word-break: break-word;
  }
  .dns-row-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
  }
  .dns-entry-count {
    font-size: 0.75rem;
    color: #6f6f6f;
  }
  .dns-ip {
    font-size: 0.8125rem;
    background: #f4f4f4;
    padding: 1px 6px;
    border-radius: 3px;
    color: #393939;
  }

  .dns-row-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }
  .dns-row-edit,
  .dns-row-delete {
    background: none;
    border: none;
    padding: 6px;
    cursor: pointer;
    border-radius: 4px;
    color: #525252;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .dns-row-edit:hover {
    background: #e0e0e0;
    color: #161616;
  }
  .dns-row-delete:hover {
    background: #fff1f1;
    color: #da1e28;
  }

  /* ── Picker modal ── */
  .dns-picker-info {
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
  }
  .dns-picker-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: #161616;
  }
  .dns-picker-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
  }

  /* ── Mobile ── */
  @media (max-width: 671px) {
    .dns-stats-row {
      gap: 8px;
    }
    .dns-stat-val {
      font-size: 0.8125rem;
    }
  }
</style>
