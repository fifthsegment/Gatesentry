<script lang="ts">
  import { isIPv4 } from "is-ip";
  import {
    Button,
    ComposedModal,
    DataTable,
    ModalBody,
    ModalFooter,
    ModalHeader,
    TextInput,
    Toolbar,
    ToolbarContent,
  } from "carbon-components-svelte";
  import { store } from "../store/apistore";
  import { _ } from "svelte-i18n";
  import { AddAlt, Edit, RowDelete, Save } from "carbon-icons-svelte";
  import { notificationstore } from "../store/notifications";
  import { createNotificationError } from "../lib/utils";
  import { createEventDispatcher } from "svelte";
  const dispatch = createEventDispatcher();

  let data = null;
  let domainText = "";
  let ipText = "";

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
    const filteredData = data.map((item) => {
      return { domain: item.domain, ip: item.ip };
    });

    $store.api
      .doCall(url, "post", filteredData, { "Content-Type": "application/json" })
      .then(function (json) {
        if (json.ok == true) {
          loadAPIData();
          domainText = "";
          ipText = "";
          showForm = false;
          dispatch("updatednsinfo");
        } else if ("error" in json) {
          notificationstore.add(
            createNotificationError(
              { title: $_("Error"), subtitle: json.error },
              $_,
            ),
          );
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
    // search if the same domain already exists
    // const existingDomain = data.find((row) => row.domain === domainText);
    // const existingIp = data.find((row) => row.ip === ipText);
    // if (existingDomain) {
    //   notificationstore.add(
    //     createNotificationError(
    //       { title: $_("Error"), subtitle: $_("Domain already exists") },
    //       $_,
    //     ),
    //   );
    //   return;
    // }
    // if (existingIp) {
    //   notificationstore.add(
    //     createNotificationError(
    //       { title: $_("Error"), subtitle: $_("IP already exists") },
    //       $_,
    //     ),
    //   );
    //   return;
    // }
    if (editingRowId) {
      const row = data.find((row) => row.id === editingRowId);
      data[editingRowId - 1].domain = domainText;
      data[editingRowId - 1].ip = ipText;

      editingRowId = null;
    } else {
      data = [...data, { id: data.length + 1, domain: domainText, ip: ipText }];
    }

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

<div>
  {#if data == null}
    <p>Loading...</p>
  {:else}
    <h5>{$_("Custom A Records")}</h5>
    <p>
      {$_("Add all domains that you would like to resolve internally here.")}
    </p>
    <br />
    {#if showForm}
      <ComposedModal
        open
        preventCloseOnClickOutside={true}
        on:submit={() => {
          addSaveRow();
        }}
        on:close={() => {
          showForm = false;
          domainText = "";
          ipText = "";
        }}
      >
        <ModalHeader title={$_("Add an entry")} />
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
          {#if showForm}{:else}
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
              icon={editingRowId != null && row.id === editingRowId
                ? Save
                : Edit}
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
</div>
