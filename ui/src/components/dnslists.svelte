<script lang="ts">
  import { onMount } from "svelte";
  import { store } from "../store/apistore";
  import {
    Button,
    ComposedModal,
    DataTable,
    ModalBody,
    ModalFooter,
    ModalHeader,
    TextInput,
  } from "carbon-components-svelte";
  import { AddAlt, Edit, RowDelete, Save } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { notificationstore } from "../store/notifications";
  import {
    buildNotificationError,
    buildNotificationSuccess,
  } from "../lib/utils";
  let data = null;
  let editingRowId = null;
  let editingItemValue = "";
  let showForm = false;
  onMount(async () => {
    const json = await $store.api.getSetting("dns_custom_entries");
    if (json) data = JSON.parse(json.Value) as Array<string>;
  });

  const saveAPIData = async () => {
    try {
      const response = await $store.api.setSetting(
        "dns_custom_entries",
        JSON.stringify(data),
      );

      notificationstore.add(
        buildNotificationSuccess(
          {
            title: $_("Success"),
            subtitle: $_("Block list updated"),
          },
          $_,
        ),
      );
    } catch (error) {
      notificationstore.add(
        buildNotificationError(
          {
            title: $_("Error"),
            subtitle: $_("Unable to save block list"),
          },
          $_,
        ),
      );
    }
  };

  const addRow = () => {
    showForm = true;
  };

  const editRow = (id: number) => {
    if (editingRowId && editingRowId === id) {
      // save the data
      data = data.map((item, index) => (item == id ? editingItemValue : item));
      saveAPIData();
      editingRowId = null;
      editingItemValue = "";
      return;
    }
    editingRowId = id;
    editingItemValue = data.find((item) => item === id);
  };

  const removeRow = (id: number) => {
    data = data.filter((item) => item !== id);
    saveAPIData();
  };

  const handleContentChange = (id, event) => {
    const newValue = event.detail;
    editingItemValue = newValue;
  };

  const addSaveRow = async () => {
    if (editingItemValue) {
      data = [...data, editingItemValue];
      await saveAPIData();
      editingItemValue = "";
      showForm = false;
    }
  };
</script>

<h5>{$_("DNS Block lists")}</h5>
<p>
  {$_(
    "A block list is a file containing a list of domains to block. Gatesentry comes with a series of predefined blocklists for adblocking. You can also add your own custom block lists or remove the existing ones.",
  )}
</p>

{#if data}
  {#if showForm}
    <ComposedModal
      open
      preventCloseOnClickOutside={true}
      on:submit={() => {
        addSaveRow();
      }}
      on:close={() => {
        showForm = false;
        editingItemValue = "";
      }}
    >
      <ModalHeader title={$_("Add a Block list")} />
      <ModalBody hasForm>
        <TextInput
          labelText={$_("Block list URL")}
          type="text"
          bind:value={editingItemValue}
          placeholder="domain.com/blocklist.txt"
          size="sm"
        />
      </ModalBody>
      <ModalFooter
        primaryButtonDisabled={false}
        primaryButtonIcon={Save}
        primaryButtonText={$_("Save")}
      />
    </ComposedModal>
  {/if}
  <DataTable
    sortable
    size="medium"
    style="width:100%;"
    headers={[
      {
        key: "content",
        value: $_("Block list URL"),
      },
      {
        key: "actions",
        value: $_("Actions"),
      },
    ]}
    rows={data
      .map((item) => {
        return {
          id: item,
          content: item,
          actions: "",
        };
      })
      .sort((a, b) => b.id - a.id)}
  >
    <div>
      <div style="float:right;">
        <Button size="small" icon={AddAlt} on:click={addRow}>
          {$_("Insert")}
        </Button>
      </div>
    </div>
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "actions"}
        <div style="float:right; width: 100px;">
          <Button
            icon={editingRowId != null && row.id === editingRowId ? Save : Edit}
            iconDescription={$_("Edit")}
            on:click={() => editRow(row.id)}
          ></Button>
          <Button
            icon={RowDelete}
            iconDescription={$_("Delete")}
            on:click={() => removeRow(row.id)}
          ></Button>
        </div>
      {:else if editingRowId && editingRowId === row.id}
        <TextInput
          value={cell.value}
          on:input={(e) => handleContentChange(row.id, e)}
        />
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
{/if}
