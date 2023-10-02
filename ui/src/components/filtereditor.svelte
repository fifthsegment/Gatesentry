<script lang="ts">
  export let filterId;
  export let title;
  export let description;
  export let showColumns = ["content", "score", "actions"];

  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    Column,
    DataTable,
    Grid,
    Row,
    TextInput,
    Tile,
    Toolbar,
    ToolbarContent,
    ToolbarSearch,
  } from "carbon-components-svelte";
  import { AddAlt, Edit, RowDelete, Save, TaskAdd } from "carbon-icons-svelte";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";
  import { _ } from "svelte-i18n";

  let data = [];

  let editingRowId = null;

  let url = `/filters/${filterId}`;

  const loadAPIdata = () => {
    $store.api
      .doCall(url)
      .then(function (content) {
        try {
          var filterData = content[0];
          data = filterData.Entries.map((item, index) => {
            return { id: index + 1, content: item.Content, score: item.Score };
          });
        } catch (err) {
          notificationstore.add({
            kind: "error",
            title: "Error:",
            subtitle: err.message,
            timeout: 30000,
          });
        }
      })
      .catch(function (err) {
        notificationstore.add({
          kind: "error",
          title: "Error:",
          subtitle: "Unable to load data from the api : " + err.message,
          timeout: 30000,
        });
      });
  };

  function handleContentChange(id, event) {
    const newValue = event.detail;
    data = data.map((item) =>
      item.id === id ? { ...item, content: newValue } : item,
    );
  }

  function handleScoreChange(id, event) {
    const newScore = parseInt(event.detail);
    if (!isNaN(newScore)) {
      data = data.map((item) =>
        item.id === id ? { ...item, score: newScore } : item,
      );
    } else {
      notificationstore.add({
        kind: "error",
        title: "Error:",
        subtitle: "Score must be a number",
        timeout: 30000,
      });
    }
  }

  const saveData = () => {
    let payload = data.map((item) => {
      return { Content: item.content, Score: item.score };
    });

    $store.api.doCall(url, "post", payload).then(function (content) {
      if (content.Response.includes("Ok")) {
        notificationstore.add({
          kind: "success",
          title: "Success:",
          subtitle: "Filter saved successfully",
          timeout: 3000,
        });
        loadAPIdata();
      }
    });
  };

  const editRow = (id) => {
    if (editingRowId == id) {
      editingRowId = null;
      saveData();
    } else {
      editingRowId = id;
    }
  };

  const removeRow = (id) => {
    data = data.filter((item) => item.id !== id);
    saveData();
  };

  const addRow = () => {
    const newId = data.length + 1;
    data = [...data, { id: newId, content: "New item", score: 0 }];
    editRow(newId);
  };

  loadAPIdata();
  let filteredRowIds = [];

  
</script>

<Grid>
  <Row>
    <Column>
      <Breadcrumb style="margin-bottom: 10px;">
        <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
        <BreadcrumbItem >{$_("Filters")}</BreadcrumbItem>
      </Breadcrumb>
      <h2>{title}</h2>
    </Column>
  </Row>
  <Row>
    <Column>
      <div style="margin: 20px 0px;">
        {description}
      </div>
    </Column>
  </Row>
  <Row>
    <Column>
      <DataTable
        sortable
        size="medium"
        style="width:100%;"
        headers={[
          {
            key: "content",
            value: "Content",
          },
          {
            key: "score",
            value: "Score",
            width: "15%",
          },
          {
            key: "actions",
            value: "Actions",
            width: "15%",
          },
        ].filter((item) => showColumns.includes(item.key))}
        rows={data.map((item) => {
          return {
            id: item.id,
            content: item.content,
            score: item.score,
            actions: "",
          };
        }).sort((a, b) => b.id - a.id)}
      >
        <Toolbar size="sm">
          <ToolbarContent>
            <ToolbarSearch value="" shouldFilterRows bind:filteredRowIds />
            <Button icon={AddAlt} on:click={addRow}>{$_("Add")}</Button>
          </ToolbarContent>
        </Toolbar>
        <svelte:fragment slot="cell" let:row let:cell>
          {#if cell.key === "actions"}
            <Button
              icon={(editingRowId != null && row.id === editingRowId ) ? Save : Edit}
              iconDescription={$_("Edit")}
              on:click={() => editRow(row.id)}
            >
          </Button>
            <Button
              icon={RowDelete}
              iconDescription={$_("Delete")}
              on:click={() => removeRow(row.id)}
            ></Button>
          {:else if editingRowId && editingRowId === row.id}
            {#if cell.key === "score"}
              <TextInput
                type="number"
                value={cell.value}
                on:input={(e) => handleScoreChange(row.id, e)}
              />
            {:else}
              <TextInput
                value={cell.value}
                on:input={(e) => handleContentChange(row.id, e)}
              />
            {/if}
          {:else}
            {cell.value}
          {/if}
        </svelte:fragment>
      </DataTable>
      {#if data.length == 0}
        <div>
          <Tile class="text-center">
            <h3>{$_("No items")}</h3>
            <div on:click={addRow} class="add-row-empty-state">
              {$_("No items yet. Click the add button below to create one. ")}
            </div>
            <div class="add-icon">
              <TaskAdd size="200" />
              {editingRowId}
            </div>
            <Button icon={AddAlt} on:click={addRow}>Create Item</Button>
          </Tile>
        </div>
      {/if}
    </Column>
  </Row>
</Grid>

<style>
  .add-icon {
    margin-bottom: 20px;
  }
  .add-row-empty-state {
    margin-top: 20px;
    margin-bottom: 20px;
  }
</style>
