<script lang="ts">
  import {
    Button,
    CodeSnippet,
    DataTable,
    InlineNotification,
    Link,
    OverflowMenu,
    OverflowMenuItem,
    TextInput,
  } from "carbon-components-svelte";
  import { AddAlt } from "carbon-icons-svelte";
  import { createEventDispatcher, onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConfirmDialog from "./ConfirmDialog.svelte";
  import Modal from "./modal.svelte";
  import ResourceState from "./layout/ResourceState.svelte";
  import SectionPanel from "./layout/SectionPanel.svelte";
  import { createNotificationError, createNotificationSuccess } from "../lib/utils";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";

  type BlockListRow = {
    id: number;
    content: string;
  };

  const dispatch = createEventDispatcher();
  const headers = [
    { key: "content", value: $_("Block-list URL") },
    { key: "actions", value: $_("Actions") },
  ];

  let data: string[] = [];
  let loading = true;
  let saving = false;
  let loadError = "";
  let editingRowId: number | null = null;
  let editingItemValue = "";
  let pendingDelete: BlockListRow | null = null;
  let showForm = false;

  $: rows = data.map((content, id) => ({ id, content }));
  $: valueIsValid = /^https?:\/\/\S+$/i.test(editingItemValue.trim());

  const loadAPIData = async () => {
    loading = true;
    loadError = "";
    try {
      const json = await $store.api.getSetting("dns_custom_entries");
      const parsed = JSON.parse(json?.Value || "[]");
      data = Array.isArray(parsed) ? parsed.map(String) : [];
    } catch (caught) {
      data = [];
      loadError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("DNS block lists could not be loaded.");
    } finally {
      loading = false;
    }
  };

  const closeForm = () => {
    showForm = false;
    editingRowId = null;
    editingItemValue = "";
  };

  const openCreate = () => {
    closeForm();
    showForm = true;
  };

  const editRow = (id: number) => {
    const value = data[id];
    if (value == null) return;
    editingRowId = id;
    editingItemValue = value;
    showForm = true;
  };

  const persist = async (next: string[]) => {
    if (saving) return false;
    saving = true;
    try {
      const response = await $store.api.setSetting(
        "dns_custom_entries",
        JSON.stringify(next),
      );
      if (response === false) throw new Error($_("The block-list setting was rejected."));

      data = next;
      dispatch("updatednsinfo");
      notificationstore.add(
        createNotificationSuccess(
          { title: $_("Block lists updated"), subtitle: $_("DNS block-list sources were saved.") },
          $_,
        ),
      );
      return true;
    } catch (caught) {
      notificationstore.add(
        createNotificationError(
          {
            title: $_("Block lists not saved"),
            subtitle:
              caught instanceof Error && caught.message
                ? caught.message
                : $_("Unable to save block lists."),
          },
          $_,
        ),
      );
      return false;
    } finally {
      saving = false;
    }
  };

  const saveRow = async () => {
    const value = editingItemValue.trim();
    if (!valueIsValid || saving) return;
    const next = editingRowId == null
      ? [...data, value]
      : data.map((item, index) => (index === editingRowId ? value : item));
    if (await persist(next)) closeForm();
  };

  const confirmDelete = async () => {
    if (!pendingDelete || saving) return;
    const target = pendingDelete;
    if (await persist(data.filter((_, index) => index !== target.id))) {
      pendingDelete = null;
    }
  };

  onMount(loadAPIData);
</script>

<SectionPanel
  title={$_("DNS block lists")}
  description={$_(
    "GateSentry downloads these domain lists and applies them to DNS requests from every protected client.",
  )}
>
  <svelte:fragment slot="actions">
    <Button size="small" icon={AddAlt} on:click={openCreate}>
      {$_("Add block list")}
    </Button>
  </svelte:fragment>

  <div class="format-guidance">
    <p>
      {$_("Each source may contain either hosts-file entries or one domain per line.")}
      <Link href="https://github.com/hagezi/dns-blocklists#fake" target="_blank" inline>
        {$_("Browse compatible lists")}
      </Link>
    </p>
    <div class="format-examples" aria-label={$_("Supported block-list formats")}>
      <CodeSnippet type="single">0.0.0.0 domain.com</CodeSnippet>
      <CodeSnippet type="single">domain.com</CodeSnippet>
    </div>
  </div>

  {#if loadError}
    <InlineNotification
      kind="error"
      title={$_("Block lists unavailable")}
      subtitle={loadError}
      on:close={() => (loadError = "")}
    />
  {/if}

  {#if loading && data.length === 0}
    <ResourceState state="loading" message={$_("Loading DNS block lists…")} />
  {:else if data.length === 0}
    <ResourceState
      state="empty"
      title={$_("No block-list sources")}
      message={$_("Add a compatible HTTP or HTTPS list to begin blocking its domains.")}
    >
      <Button size="small" icon={AddAlt} on:click={openCreate}>
        {$_("Add block list")}
      </Button>
    </ResourceState>
  {:else}
    <div class="table-region">
      <DataTable sortable size="compact" {headers} {rows}>
        <svelte:fragment slot="cell" let:row let:cell>
          {#if cell.key === "actions"}
            <OverflowMenu flipped iconDescription={$_("Block-list actions")}>
              <OverflowMenuItem text={$_("Edit")} on:click={() => editRow(Number(row.id))} />
              <OverflowMenuItem
                danger
                text={$_("Delete")}
                on:click={() => (pendingDelete = rows.find((item) => item.id === Number(row.id)) ?? null)}
              />
            </OverflowMenu>
          {:else}
            <span class="url-cell">{cell.value}</span>
          {/if}
        </svelte:fragment>
      </DataTable>
    </div>
  {/if}
</SectionPanel>

<Modal
  bind:open={showForm}
  title={editingRowId == null ? $_("Add DNS block list") : $_("Edit DNS block list")}
  label={$_("DNS")}
  primaryButtonText={saving ? $_("Saving…") : $_("Save block list")}
  secondaryButtonText={$_("Cancel")}
  primaryButtonDisabled={!valueIsValid || saving}
  hasForm
  shouldSubmitOnEnter
  preventCloseOnClickOutside={saving}
  on:submit={saveRow}
  on:close={closeForm}
>
  <TextInput
    labelText={$_("Block-list URL")}
    helperText={$_("Use a complete HTTP or HTTPS URL to a plain-text domain list.")}
    invalid={Boolean(editingItemValue) && !valueIsValid}
    invalidText={$_("Enter a complete HTTP or HTTPS URL.")}
    type="url"
    bind:value={editingItemValue}
    placeholder="https://example.org/blocklist.txt"
    disabled={saving}
  />
</Modal>

<ConfirmDialog
  open={pendingDelete != null}
  title={$_("Remove DNS block list?")}
  label={$_("DNS")}
  confirmText={$_("Remove block list")}
  cancelText={$_("Cancel")}
  danger
  busy={saving}
  on:submit={confirmDelete}
  on:close={() => (pendingDelete = null)}
>
  <p>
    {$_("GateSentry will stop downloading and applying")}
    <strong>{pendingDelete?.content ?? ""}</strong>.
  </p>
</ConfirmDialog>

<style>
  .format-guidance {
    display: grid;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .format-guidance p {
    max-width: 48rem;
    margin: 0;
    color: var(--cds-text-secondary, #525252);
    line-height: 1.45;
  }

  .format-guidance :global(.bx--link) {
    margin-left: 0.25rem;
  }

  .format-examples {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
  }

  .table-region {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
  }

  .url-cell {
    overflow-wrap: anywhere;
  }

  @media (max-width: 42rem) {
    .format-examples {
      grid-template-columns: 1fr;
    }
  }
</style>
