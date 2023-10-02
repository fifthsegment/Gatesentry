<script lang="ts">
  import { isIP, isIPv4 } from "is-ip";
  import {
    Button,
    DataTable,
    FormLabel,
    TextInput,
    Toolbar,
    ToolbarContent,
    ToolbarSearch,
  } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  import { AddAlt, Edit, RowDelete, Save } from "carbon-icons-svelte";
  import { notificationstore } from "../../store/notifications";
  let data = null;
  let domainText = "";
  let ipText = "";
  let isValid = false;

  let editingRowId = null;
  let showForm = false;

  const url = "/dns/custom_entries";

  const loadAPIData = () => {
    data = [];
    $store.api.doCall(url).then(function (json) {
      data = json.data.map((item, index) => {
        return { ...item, id: index + 1 };
      });
    });
  };

  const saveAPIData = () => {
    $store.api
      .doCall(url, "post", data)
      .then(function (json) {
        if (json.ok == true) {
          loadAPIData();
        }
      })
      .catch(function (err) {
        notificationstore.add({
          kind: "error",
          title: "Error:",
          subtitle: "Unable to save data to the api : " + err.message,
          timeout: 30000,
        });
        loadAPIData();
      });
  };

  loadAPIData();

  const addSaveRow = () => {
    if (editingRowId) {
      const row = data.find((row) => row.id === editingRowId);
      data[editingRowId - 1].domain = domainText;
      data[editingRowId - 1].ip = ipText;

      editingRowId = null;
    } else {
      data = [...data, { id: data.length + 1, domain: domainText, ip: ipText }];
    }
    domainText = "";
    ipText = "";
    showForm = false;
    saveAPIData();
  };

  const editRow = (rowId) => {
    editingRowId = rowId;
    const row = data.find((row) => row.id === rowId);
    domainText = row.domain;
    ipText = row.ip;
    showForm = true;
    saveAPIData();
  };

  const deleteRow = (rowId) => {
    data = [...data.filter((row) => row.id !== rowId)];
    saveAPIData();
  };
</script>

<h2>Custom Entries</h2>
<br />
{#if data == null}
  <p>Loading...</p>
{:else}
  <DataTable
    sortable
    size="medium"
    style="width:100%;"
    headers={[
      {
        key: "domain",
        value: "Domain",
      },
      {
        key: "ip",
        value: "IP",
      },
      {
        key: "actions",
        value: "",
      },
    ]}
    rows={data
      .map((item, index) => {
        return {
          ...item,
          actions: "",
        };
      })
      .sort((a, b) => b.id - a.id)}
  >
    <Toolbar size="sm">
      <ToolbarContent>
        {#if showForm}
          <TextInput
            type="text"
            bind:value={domainText}
            placeholder="Domain"
            size="sm"
          />
          <TextInput
            type="text"
            bind:value={ipText}
            placeholder="IP"
            size="sm"
          />
          <Button icon={Save} on:click={addSaveRow} disabled={!isIPv4(ipText)}>
            {editingRowId != null ? $_("Update") : $_("Add")}
          </Button>
        {:else}
          <Button icon={AddAlt} on:click={() => (showForm = true)}>
            {$_("Insert")}
          </Button>
        {/if}
      </ToolbarContent>
    </Toolbar>
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "actions"}
        <div style="float:right;">
          <Button
            icon={editingRowId != null && row.id === editingRowId ? Save : Edit}
            iconDescription={$_("Edit")}
            disabled={row.id == editingRowId}
            on:click={() => editRow(row.id)}
          />
          <Button
            icon={RowDelete}
            iconDescription={$_("Delete")}
            on:click={() => deleteRow(row.id)}
          />
        </div>
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
{/if}
