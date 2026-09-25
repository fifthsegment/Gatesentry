<script lang="ts">
  import { isIPv4 } from "is-ip";
  import {
    Button,
    DataTable,
    InlineNotification,
    OverflowMenu,
    OverflowMenuItem,
    TextInput,
    Toolbar,
    ToolbarContent,
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

  type DnsRecord = {
    id: number;
    domain: string;
    ip: string;
  };

  const dispatch = createEventDispatcher();
  const url = "/dns/custom_entries";
  const headers = [
    { key: "domain", value: $_("Domain") },
    { key: "ip", value: $_("IP address") },
    { key: "actions", value: $_("Actions") },
  ];

  let data: DnsRecord[] = [];
  let loading = true;
  let saving = false;
  let loadError = "";
  let domainText = "";
  let ipText = "";
  let editingRowId: number | null = null;
  let pendingDelete: DnsRecord | null = null;
  let showForm = false;

  const loadAPIData = async () => {
    loading = true;
    loadError = "";
    try {
      const json = await $store.api.doCall(url);
      const records = Array.isArray(json?.data) ? json.data : [];
      data = records.map((item, index) => ({
        id: index + 1,
        domain: String(item.domain ?? ""),
        ip: String(item.ip ?? ""),
      }));
    } catch (caught) {
      loadError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("Custom DNS records could not be loaded.");
    } finally {
      loading = false;
    }
  };

  const closeForm = () => {
    showForm = false;
    editingRowId = null;
    domainText = "";
    ipText = "";
  };

  const openCreate = () => {
    closeForm();
    showForm = true;
  };

  const editRow = (rowId: number) => {
    const row = data.find((item) => item.id === rowId);
    if (!row) return;
    editingRowId = rowId;
    domainText = row.domain;
    ipText = row.ip;
    showForm = true;
  };

  const persist = async (records: DnsRecord[]) => {
    if (saving) return false;
    saving = true;
    try {
      const payload = records.map(({ domain, ip }) => ({ domain, ip }));
      const json = await $store.api.doCall(url, "post", payload, {
        "Content-Type": "application/json",
      });
      if (!json?.ok) {
        throw new Error(json?.error || $_("The DNS records were not accepted."));
      }

      notificationstore.add(
        createNotificationSuccess(
          { title: $_("DNS records updated"), subtitle: $_("Custom A records were saved.") },
          $_,
        ),
      );
      dispatch("updatednsinfo");
      await loadAPIData();
      return true;
    } catch (caught) {
      notificationstore.add(
        createNotificationError(
          {
            title: $_("DNS records not saved"),
            subtitle:
              caught instanceof Error && caught.message
                ? caught.message
                : $_("Custom DNS records could not be saved."),
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
    const domain = domainText.trim();
    const ip = ipText.trim();
    if (!domain || !isIPv4(ip) || saving) return;

    const next = editingRowId == null
      ? [...data, { id: data.length + 1, domain, ip }]
      : data.map((row) =>
          row.id === editingRowId ? { ...row, domain, ip } : row,
        );

    if (await persist(next)) closeForm();
  };

  const confirmDelete = async () => {
    if (!pendingDelete || saving) return;
    const target = pendingDelete;
    const saved = await persist(data.filter((row) => row.id !== target.id));
    if (saved) pendingDelete = null;
  };

  onMount(loadAPIData);
</script>

<SectionPanel
  title={$_("Custom A records")}
  description={$_("Resolve selected domains to local IPv4 addresses before using the upstream resolver.")}
>
  <svelte:fragment slot="actions">
    <Button size="small" icon={AddAlt} on:click={openCreate}>
      {$_("Add record")}
    </Button>
  </svelte:fragment>

  {#if loadError}
    <InlineNotification
      kind="error"
      title={$_("Custom records unavailable")}
      subtitle={loadError}
      on:close={() => (loadError = "")}
    />
  {/if}

  {#if loading && data.length === 0}
    <ResourceState state="loading" message={$_("Loading custom DNS records…")} />
  {:else if data.length === 0}
    <ResourceState
      state="empty"
      title={$_("No custom records")}
      message={$_("Add a record when a local domain should resolve to a specific IPv4 address.")}
    />
  {:else}
    <div class="table-region">
      <DataTable sortable size="compact" {headers} rows={data}>
        <Toolbar size="sm">
          <ToolbarContent />
        </Toolbar>
        <svelte:fragment slot="cell" let:row let:cell>
          {#if cell.key === "actions"}
            <OverflowMenu flipped iconDescription={$_("Record actions")}>
              <OverflowMenuItem text={$_("Edit")} on:click={() => editRow(row.id)} />
              <OverflowMenuItem
                danger
                text={$_("Delete")}
                on:click={() => (pendingDelete = data.find((item) => item.id === row.id) ?? null)}
              />
            </OverflowMenu>
          {:else}
            {cell.value}
          {/if}
        </svelte:fragment>
      </DataTable>
    </div>
  {/if}
</SectionPanel>

<Modal
  bind:open={showForm}
  title={editingRowId == null ? $_("Add custom record") : $_("Edit custom record")}
  label={$_("DNS")}
  primaryButtonText={saving ? $_("Saving…") : $_("Save record")}
  secondaryButtonText={$_("Cancel")}
  primaryButtonDisabled={!domainText.trim() || !isIPv4(ipText.trim()) || saving}
  hasForm
  shouldSubmitOnEnter
  preventCloseOnClickOutside={saving}
  on:submit={saveRow}
  on:close={closeForm}
>
  <div class="record-form">
    <TextInput
      labelText={$_("Domain")}
      type="text"
      bind:value={domainText}
      placeholder="printer.home"
      disabled={saving}
    />
    <TextInput
      labelText={$_("IPv4 address")}
      helperText={ipText && !isIPv4(ipText.trim()) ? $_("Enter a valid IPv4 address.") : ""}
      invalid={Boolean(ipText) && !isIPv4(ipText.trim())}
      invalidText={$_("Enter a valid IPv4 address.")}
      type="text"
      bind:value={ipText}
      placeholder="192.168.1.20"
      disabled={saving}
    />
  </div>
</Modal>

<ConfirmDialog
  open={pendingDelete != null}
  title={$_("Delete custom DNS record?")}
  label={$_("DNS")}
  confirmText={$_("Delete record")}
  cancelText={$_("Cancel")}
  danger
  busy={saving}
  on:submit={confirmDelete}
  on:close={() => (pendingDelete = null)}
>
  <p>
    {$_("GateSentry will stop resolving")}
    <strong>{pendingDelete?.domain ?? ""}</strong>
    {$_("to the configured local address.")}
  </p>
</ConfirmDialog>

<style>
  .table-region {
    min-width: 0;
    max-width: 100%;
    overflow-x: auto;
  }

  .record-form {
    display: grid;
    gap: 1rem;
  }
</style>
