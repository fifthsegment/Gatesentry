<script lang="ts">
  import {
    Button,
    DataTable,
    InlineNotification,
    OverflowMenu,
    OverflowMenuItem,
    TextInput,
    Toolbar,
    ToolbarContent,
    ToolbarSearch,
  } from "carbon-components-svelte";
  import { AddAlt } from "carbon-icons-svelte";
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConfirmDialog from "./ConfirmDialog.svelte";
  import ResourceState from "./layout/ResourceState.svelte";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";

  type FilterRow = {
    id: number;
    content: string;
    score: number;
    draft?: boolean;
  };

  export let filterId: string;
  export let showColumns: string[] = ["content", "score", "actions"];

  const allHeaders = [
    { key: "content", value: $_("Content") },
    { key: "score", value: $_("Score") },
    { key: "actions", value: $_("Actions") },
  ];

  let data: FilterRow[] = [];
  let loading = true;
  let saving = false;
  let loadError = "";
  let saveError = "";
  let editingRowId: number | null = null;
  let pendingDelete: FilterRow | null = null;
  let filteredRowIds: Array<string | number> = [];
  let nextId = 1;

  $: headers = allHeaders.filter((item) => showColumns.includes(item.key));
  $: rows = data.map(({ id, content, score }) => ({ id, content, score }));
  $: editingRow = data.find((item) => item.id === editingRowId) ?? null;
  $: canSaveEditing = Boolean(editingRow?.content.trim()) &&
    (!showColumns.includes("score") || Number.isFinite(editingRow?.score));

  const loadAPIData = async () => {
    loading = true;
    loadError = "";
    try {
      const content = await $store.api.doCall(`/filters/${filterId}`);
      const filterData = Array.isArray(content) ? content[0] : null;
      const entries = Array.isArray(filterData?.Entries) ? filterData.Entries : [];
      data = entries.map((item, index) => ({
        id: index + 1,
        content: String(item.Content ?? ""),
        score: Number(item.Score) || 0,
      }));
      nextId = data.length + 1;
      editingRowId = null;
    } catch (caught) {
      loadError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("This inspection list could not be loaded.");
    } finally {
      loading = false;
    }
  };

  const notifySaved = () => {
    notificationstore.add({
      kind: "success",
      title: $_("Inspection list updated"),
      subtitle: $_("The filter was saved successfully."),
      timeout: 3000,
    });
  };

  const saveData = async (next: FilterRow[]) => {
    if (saving) return false;
    saving = true;
    saveError = "";
    try {
      const payload = next.map((item) => ({
        Content: item.content,
        Score: item.score,
      }));
      const content = await $store.api.doCall(`/filters/${filterId}`, "post", payload);
      const response = String(content?.response ?? content?.Response ?? "");
      if (!response.includes("Ok")) {
        throw new Error(content?.error || $_("The filter service did not confirm the save."));
      }
      notifySaved();
      await loadAPIData();
      return true;
    } catch (caught) {
      saveError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("This inspection list could not be saved.");
      return false;
    } finally {
      saving = false;
    }
  };

  const handleContentChange = (id: number, event: CustomEvent<string | number>) => {
    const value = String(event.detail ?? "");
    data = data.map((item) => (item.id === id ? { ...item, content: value } : item));
  };

  const handleScoreChange = (id: number, event: CustomEvent<string | number>) => {
    const value = Number(event.detail);
    data = data.map((item) => (item.id === id ? { ...item, score: value } : item));
  };

  const editRow = (id: number) => {
    editingRowId = id;
    saveError = "";
  };

  const saveEditingRow = async () => {
    if (!editingRow || !canSaveEditing) return;
    await saveData(data.map((item) => ({ ...item, draft: false })));
  };

  const cancelEditing = () => {
    if (editingRow?.draft) data = data.filter((item) => item.id !== editingRow.id);
    editingRowId = null;
    saveError = "";
  };

  const addRow = () => {
    if (editingRowId != null) return;
    const row: FilterRow = {
      id: nextId++,
      content: "",
      score: 0,
      draft: true,
    };
    data = [...data, row];
    editingRowId = row.id;
  };

  const confirmDelete = async () => {
    if (!pendingDelete || saving) return;
    const target = pendingDelete;
    if (await saveData(data.filter((item) => item.id !== target.id))) {
      pendingDelete = null;
    }
  };

  onMount(loadAPIData);
</script>

{#if loadError}
  <InlineNotification
    kind="error"
    title={$_("Inspection list unavailable")}
    subtitle={loadError}
    on:close={() => (loadError = "")}
  />
{/if}
{#if saveError}
  <InlineNotification
    kind="error"
    title={$_("Inspection list not saved")}
    subtitle={saveError}
    on:close={() => (saveError = "")}
  />
{/if}

{#if loading && data.length === 0}
  <ResourceState state="loading" message={$_("Loading inspection list…")} />
{:else if data.length === 0 && editingRowId == null}
  <ResourceState
    state="empty"
    title={$_("No list entries")}
    message={$_("Add an entry when this inspection rule should match content or a host.")}
  >
    <Button size="small" icon={AddAlt} on:click={addRow}>{$_("Add entry")}</Button>
  </ResourceState>
{:else}
  <div class="filter-table">
    <DataTable sortable size="compact" {headers} {rows}>
      <Toolbar size="sm">
        <ToolbarContent>
          <ToolbarSearch
            value=""
            shouldFilterRows
            bind:filteredRowIds
            placeholder={$_("Search entries")}
          />
          <Button
            size="small"
            icon={AddAlt}
            disabled={editingRowId != null || saving}
            on:click={addRow}
          >
            {$_("Add entry")}
          </Button>
        </ToolbarContent>
      </Toolbar>
      <svelte:fragment slot="cell" let:row let:cell>
        {#if cell.key === "actions"}
          {#if editingRowId === Number(row.id)}
            <div class="edit-actions">
              <Button
                size="small"
                disabled={!canSaveEditing || saving}
                on:click={saveEditingRow}
              >
                {saving ? $_("Saving…") : $_("Save")}
              </Button>
              <Button kind="ghost" size="small" disabled={saving} on:click={cancelEditing}>
                {$_("Cancel")}
              </Button>
            </div>
          {:else}
            <OverflowMenu flipped iconDescription={$_("Entry actions")}>
              <OverflowMenuItem text={$_("Edit")} on:click={() => editRow(Number(row.id))} />
              <OverflowMenuItem
                danger
                text={$_("Delete")}
                on:click={() => (pendingDelete = data.find((item) => item.id === Number(row.id)) ?? null)}
              />
            </OverflowMenu>
          {/if}
        {:else if editingRowId === Number(row.id)}
          {#if cell.key === "score"}
            <TextInput
              type="number"
              labelText={$_("Score")}
              hideLabel
              invalid={!Number.isFinite(Number(cell.value))}
              invalidText={$_("Score must be a number.")}
              value={cell.value}
              on:input={(event) => handleScoreChange(Number(row.id), event)}
            />
          {:else}
            <TextInput
              labelText={$_("Content")}
              hideLabel
              invalid={!String(cell.value).trim()}
              invalidText={$_("Content is required.")}
              value={cell.value}
              on:input={(event) => handleContentChange(Number(row.id), event)}
            />
          {/if}
        {:else}
          {cell.value}
        {/if}
      </svelte:fragment>
    </DataTable>
  </div>
{/if}

<ConfirmDialog
  open={pendingDelete != null}
  title={$_("Delete inspection-list entry?")}
  label={$_("HTTPS inspection")}
  confirmText={$_("Delete entry")}
  cancelText={$_("Cancel")}
  danger
  busy={saving}
  on:submit={confirmDelete}
  on:close={() => (pendingDelete = null)}
>
  <p>
    {$_("Remove")}
    <strong>{pendingDelete?.content ?? ""}</strong>
    {$_("from this inspection list?")}
  </p>
</ConfirmDialog>

<style>
  .filter-table {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
  }

  .edit-actions {
    display: flex;
    gap: 0.25rem;
    flex-wrap: wrap;
  }

  :global(.filter-table .bx--toolbar-content) {
    min-width: 0;
    flex-wrap: wrap;
  }
</style>
