<script lang="ts">
  import { createEventDispatcher, onMount } from "svelte";
  import { store } from "../store/apistore";
  import {
    Button,
    ComposedModal,
    DataTable,
    InlineLoading,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Tag,
    Toolbar,
    ToolbarContent,
  } from "carbon-components-svelte";
  import { AddAlt, RowDelete } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  import { getBasePath } from "../lib/navigate";
  const dispatch = createEventDispatcher();

  // All available domain lists from the API
  let allLists = [];

  // Currently assigned list IDs
  let blockListIds: string[] = [];
  let allowListIds: string[] = [];

  // Picker state
  let showPicker = false;
  let pickerMode: "block" | "allow" = "block";

  let loaded = false;

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

  onMount(async () => {
    await Promise.all([loadAllLists(), loadAssignedIds()]);
    loaded = true;
  });

  function findList(id: string) {
    return allLists.find((l) => l.id === id);
  }

  // Build display rows from IDs + allLists metadata
  function buildRows(ids: string[]) {
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

  // Available lists for the picker (not already assigned to this mode)
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
      dispatch("updatednsinfo");
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
      dispatch("updatednsinfo");
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
</script>

{#if showPicker}
  <ComposedModal
    open
    on:close={() => {
      showPicker = false;
    }}
  >
    <ModalHeader
      title={pickerMode === "block"
        ? $_("Add Domain List to Block List")
        : $_("Add Domain List to Allow List")}
    />
    <ModalBody>
      {#if getAvailableForPicker().length === 0}
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
          rows={getAvailableForPicker().map((l) => ({
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
                on:click={() => addListToPicker(row.id)}
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
      <Button kind="secondary" on:click={() => (showPicker = false)}>
        {$_("Cancel")}
      </Button>
    </ModalFooter>
  </ComposedModal>
{/if}

{#if !loaded}
  <InlineLoading description="Loading DNS lists..." />
{:else}
  <!-- Allow List Section -->
  <h5>{$_("DNS Allow Lists")}</h5>
  <p style="margin-bottom: 8px; color: #525252;">
    {$_(
      "Domains in allow lists are never blocked by DNS filtering, even if they appear in a block list.",
    )}
  </p>

  <DataTable
    size="medium"
    headers={[
      { key: "name", value: $_("Name") },
      { key: "source", value: $_("Source") },
      { key: "category", value: $_("Category") },
      { key: "entry_count", value: $_("Domains") },
      { key: "actions", value: "" },
    ]}
    rows={buildRows(allowListIds)}
  >
    <Toolbar size="sm">
      <ToolbarContent>
        <Button
          size="small"
          kind="primary"
          icon={AddAlt}
          on:click={() => openPicker("allow")}
        >
          {$_("Add Allow List")}
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
            on:click={() => removeAllowList(row.id)}
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

  <br />

  <!-- Block List Section -->
  <h5>{$_("DNS Block Lists")}</h5>
  <p style="margin-bottom: 8px; color: #525252;">
    {$_(
      "Domains in block lists are blocked at the DNS level. Assign domain lists here to enforce DNS-level blocking for all users.",
    )}
  </p>

  <DataTable
    size="medium"
    headers={[
      { key: "name", value: $_("Name") },
      { key: "source", value: $_("Source") },
      { key: "category", value: $_("Category") },
      { key: "entry_count", value: $_("Domains") },
      { key: "actions", value: "" },
    ]}
    rows={buildRows(blockListIds)}
  >
    <Toolbar size="sm">
      <ToolbarContent>
        <Button
          size="small"
          kind="primary"
          icon={AddAlt}
          on:click={() => openPicker("block")}
        >
          {$_("Add Block List")}
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
            on:click={() => removeBlockList(row.id)}
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
{/if}
