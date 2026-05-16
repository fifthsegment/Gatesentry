<script lang="ts">
  export let filterId;

  import {
    Button,
    InlineLoading,
    InlineNotification,
    TextInput,
  } from "carbon-components-svelte";
  import { Add, Edit, Save, TrashCan } from "carbon-icons-svelte";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";
  import { _ } from "svelte-i18n";
  import Toggle from "./toggle.svelte";

  let data = [];
  let loading = true;
  let editingRowId = null;
  let searchQuery = "";

  let url = `/filters/${filterId}`;

  let enable_https_filtering = "";

  const loadAPIdata = () => {
    loading = true;
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
          title: $_("Error:"),
          subtitle: $_("Unable to load data from the api : ") + err.message,
          timeout: 30000,
        });
      })
      .finally(() => {
        loading = false;
      });
  };

  loadAPIdata();

  $: filteredData = searchQuery
    ? data.filter((item) =>
        item.content.toLowerCase().includes(searchQuery.toLowerCase()),
      )
    : data;

  $: sortedData = filteredData.slice().sort((a, b) => b.id - a.id);

  const saveData = () => {
    let payload = data.map((item) => {
      return { Content: item.content, Score: item.score };
    });
    $store.api.doCall(url, "post", payload).then(function (content) {
      if (content.Response.includes("Ok")) {
        notificationstore.add({
          kind: "success",
          title: "Saved",
          subtitle: "Filter saved successfully",
          timeout: 3000,
        });
        loadAPIdata();
      }
    });
  };

  const editRow = (id) => {
    if (editingRowId === id) {
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
    data = [...data, { id: newId, content: $_("New item"), score: 0 }];
    editingRowId = newId;
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
    }
  }
</script>

<Toggle
  bind:settingValue={enable_https_filtering}
  settingName="enable_https_filtering"
  hide={true}
/>

{#if enable_https_filtering === "false"}
  <InlineNotification
    hideCloseButton
    kind="warning"
    title={$_("Important: ")}
    subtitle={$_(
      "For these filters to take effect, you must enable HTTPS Filtering from the Settings Menu.",
    )}
  />
{/if}

<div class="fe-topbar">
  <div class="fe-search">
    <TextInput size="sm" placeholder="Search..." bind:value={searchQuery} />
  </div>
  <div class="fe-spacer"></div>
  <Button size="small" icon={Add} kind="tertiary" on:click={addRow}>Add</Button>
</div>

{#if loading}
  <InlineLoading description="Loading..." />
{:else if sortedData.length === 0 && data.length === 0}
  <div class="gs-card">
    <p class="gs-empty" style="text-align: center; padding: 24px 0;">
      No items yet. Click <strong>Add</strong> to create one.
    </p>
  </div>
{:else if sortedData.length === 0}
  <div class="gs-card">
    <p class="gs-empty" style="text-align: center; padding: 24px 0;">
      No matches for "{searchQuery}"
    </p>
  </div>
{:else}
  <div class="fe-list">
    {#each sortedData as item (item.id)}
      <div class="fe-row" class:fe-row--editing={editingRowId === item.id}>
        {#if editingRowId === item.id}
          <div class="fe-edit-fields">
            <div class="fe-edit-content">
              <TextInput
                size="sm"
                value={item.content}
                placeholder="Content"
                on:input={(e) => handleContentChange(item.id, e)}
              />
            </div>
            <div class="fe-edit-score">
              <TextInput
                size="sm"
                type="number"
                value={String(item.score)}
                placeholder="Score"
                on:input={(e) => handleScoreChange(item.id, e)}
              />
            </div>
          </div>
        {:else}
          <div class="fe-body">
            <span class="fe-content">{item.content}</span>
            <span class="fe-score">Score: {item.score}</span>
          </div>
        {/if}

        <div class="fe-actions">
          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <span
            class="fe-icon-btn"
            role="button"
            tabindex="0"
            title={editingRowId === item.id ? $_("Save") : $_("Edit")}
            on:click={() => editRow(item.id)}
            on:keydown={(e) => {
              if (e.key === "Enter" || e.key === " ") editRow(item.id);
            }}
          >
            {#if editingRowId === item.id}
              <Save size={20} />
            {:else}
              <Edit size={20} />
            {/if}
          </span>

          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <span
            class="fe-icon-btn fe-icon-btn--danger"
            role="button"
            tabindex="0"
            title={$_("Delete")}
            on:click={() => removeRow(item.id)}
            on:keydown={(e) => {
              if (e.key === "Enter" || e.key === " ") removeRow(item.id);
            }}><TrashCan size={20} /></span
          >
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .fe-topbar {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 12px;
  }
  .fe-search {
    flex: 1;
    max-width: 300px;
    min-width: 0;
  }
  .fe-spacer {
    flex: 1;
  }

  .fe-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    background: #e0e0e0;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    overflow: hidden;
  }

  .fe-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    background: #fff;
    -webkit-tap-highlight-color: transparent;
  }

  .fe-row--editing {
    background: #f4f4f4;
  }

  .fe-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .fe-content {
    font-size: 0.875rem;
    font-weight: 500;
    color: #161616;
    word-break: break-word;
  }

  .fe-score {
    font-size: 0.75rem;
    color: #6f6f6f;
  }

  .fe-edit-fields {
    flex: 1;
    min-width: 0;
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }

  .fe-edit-content {
    flex: 1;
    min-width: 0;
  }

  .fe-edit-score {
    width: 90px;
    flex-shrink: 0;
  }

  .fe-actions {
    display: flex;
    gap: 4px;
    flex-shrink: 0;
  }

  .fe-icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    color: #525252;
    cursor: pointer;
    transition:
      background-color 0.12s,
      color 0.12s;
    -webkit-tap-highlight-color: transparent;
  }
  .fe-icon-btn:hover {
    background: #e0e0e0;
    color: #161616;
  }
  .fe-icon-btn:active {
    background: #c6c6c6;
  }
  .fe-icon-btn--danger:hover {
    background: #ffd7d9;
    color: #a2191f;
  }
  .fe-icon-btn--danger:active {
    background: #ffb3b8;
    color: #a2191f;
  }

  @media (max-width: 671px) {
    .fe-search {
      max-width: none;
    }
    .fe-row {
      padding: 10px 10px;
      gap: 8px;
    }
    .fe-icon-btn {
      width: 40px;
      height: 40px;
    }
    .fe-edit-fields {
      flex-direction: column;
      gap: 6px;
    }
    .fe-edit-score {
      width: 100%;
    }
  }
</style>
