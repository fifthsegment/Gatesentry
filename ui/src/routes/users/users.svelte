<script lang="ts">
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { Button, InlineLoading, Toggle } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { AddAlt, Edit, TrashCan, UserAccess } from "carbon-icons-svelte";
  import Usermodal from "./usermodal.svelte";
  import Modal from "../../components/modal.svelte";
  import type { UserType } from "../../types";
  import { bytesToSize } from "../../lib/utils";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";

  let showForm = false;

  let users: Array<UserType> | null = null;
  let editingUser: UserType | null = null;

  const loadUsers = async () => {
    users = null;
    users = (await $store.api.getUsers()).users ?? [];
  };

  onMount(loadUsers);

  const editUser = (username: string) => {
    editingUser = users?.find((user) => user.username === username);
    showForm = true;
  };

  const deleteUser = async (user: string) => {
    if (confirm($_("Are you sure you want to delete this user?"))) {
      await $store.api.deleteUser(user);
      await loadUsers();
    }
  };

  const handleCreateUser = async (event) => {
    showForm = false;
    await loadUsers();
  };

  const handleUpdateUser = async (event) => {
    showForm = false;
    await loadUsers();
  };

  const addUser = () => {
    editingUser = null;
    showForm = true;
  };

  const toggleAccess = async (user: UserType) => {
    const newAccess = !user.allowaccess;
    user.allowaccess = newAccess;
    users = users; // reactivity
    await $store.api.updateUser({
      username: user.username,
      password: "",
      allowaccess: newAccess,
    });
    await loadUsers();
  };
</script>

<div class="gs-page-title">
  <UserAccess size={24} />
  <h2>{$_("Users")}</h2>
</div>

<br />

<Modal
  shouldSubmitOnEnter={true}
  hasForm={true}
  title={editingUser != null ? $_("Edit User") : $_("Add User")}
  bind:open={showForm}
  on:close={() => {
    showForm = false;
    editingUser = null;
  }}
  ><Usermodal
    on:createuser={handleCreateUser}
    on:updateuser={handleUpdateUser}
    user={editingUser}
  /></Modal
>

<ConnectedSettingInput
  keyName="EnableUsers"
  type="radio"
  title={$_("Allow only registered users to access the proxy server")}
  labelText={$_("Allow only registered users to access the proxy server")}
  helperText=""
/>

{#if !users}
  <InlineLoading description="Loading users..." />
{:else}
  <div class="ul-topbar">
    <Button size="small" icon={AddAlt} kind="tertiary" on:click={addUser}
      >{$_("Add User")}</Button
    >
  </div>

  {#if users.length === 0}
    <div class="gs-card">
      <p class="gs-empty" style="text-align: center; padding: 24px 0;">
        {$_("No users registered yet.")}
      </p>
    </div>
  {:else}
    <div class="ul-list">
      {#each users as user (user.username)}
        <div class="ul-row">
          <div class="ul-body">
            <span class="ul-name">{user.username}</span>
            <span class="ul-data">{bytesToSize(user.dataconsumed)}</span>
          </div>

          <!-- svelte-ignore a11y-no-static-element-interactions a11y-click-events-have-key-events -->
          <span
            class="ul-toggle"
            on:click|stopPropagation
            on:keydown|stopPropagation
          >
            <Toggle
              size="sm"
              toggled={user.allowaccess}
              hideLabel
              labelA=""
              labelB=""
              on:toggle={() => toggleAccess(user)}
            />
          </span>

          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <span
            class="ul-icon-btn"
            role="button"
            tabindex="0"
            title={$_("Edit User")}
            on:click={() => editUser(user.username)}
            on:keydown={(e) => {
              if (e.key === "Enter" || e.key === " ") editUser(user.username);
            }}><Edit size={20} /></span
          >

          <!-- svelte-ignore a11y-no-static-element-interactions -->
          <span
            class="ul-icon-btn ul-icon-btn--danger"
            role="button"
            tabindex="0"
            title={$_("Delete")}
            on:click={() => deleteUser(user.username)}
            on:keydown={(e) => {
              if (e.key === "Enter" || e.key === " ") deleteUser(user.username);
            }}><TrashCan size={20} /></span
          >
        </div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .ul-topbar {
    display: flex;
    justify-content: flex-end;
    margin-bottom: 12px;
  }

  .ul-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    background: #e0e0e0;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    overflow: hidden;
  }

  .ul-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 14px;
    background: #fff;
    -webkit-tap-highlight-color: transparent;
  }

  .ul-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .ul-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: #161616;
    word-break: break-word;
  }

  .ul-data {
    font-size: 0.75rem;
    color: #6f6f6f;
  }

  .ul-toggle {
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }
  .ul-toggle :global(.bx--form-item) {
    flex: 0 0 auto;
  }

  .ul-icon-btn {
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
  .ul-icon-btn:hover {
    background: #e0e0e0;
    color: #161616;
  }
  .ul-icon-btn:active {
    background: #c6c6c6;
  }
  .ul-icon-btn--danger:hover {
    background: #ffd7d9;
    color: #a2191f;
  }
  .ul-icon-btn--danger:active {
    background: #ffb3b8;
    color: #a2191f;
  }

  @media (max-width: 671px) {
    .ul-row {
      padding: 10px 10px;
      gap: 8px;
    }
    .ul-icon-btn {
      width: 40px;
      height: 40px;
    }
  }
</style>
