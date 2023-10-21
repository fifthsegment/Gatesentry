<script lang="ts">
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    DataTable,
    Loading,
    TextInput,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import {
    AddAlt,
    Edit,
    Pause,
    Restart,
    RowDelete,
    Stop,
    StopFilledAlt,
  } from "carbon-icons-svelte";
  import Usermodal from "./usermodal.svelte";
  import Modal from "../../components/modal.svelte";
  import type { UserType } from "../../types";
  import { bytesToSize } from "../../lib/utils";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";

  let showForm = false;

  // type User = {
  //   user: string;
  //   dataconsumed: string;
  //   allowaccess: boolean;
  // };

  let users: Array<UserType> | null = null;
  let editingUser: UserType | null = null;
  const loadUsers = async () => {
    users = null;
    users = (await $store.api.getUsers()).users ?? [];
  };

  onMount(loadUsers);

  const editUser = (username: string) => {
    // find user in users
    editingUser = users?.find((user) => user.username === username);

    showForm = true;
  };

  const deleteUser = async (user: string) => {
    if (confirm($_("Are you sure you want to delete this user?"))) {
      await $store.api.deleteUser(user);
      await loadUsers();
    }
  };

  const handleCreateUser = async (
    event: CustomEvent<{ username: string; password: string }>,
  ) => {
    showForm = false;
    await loadUsers();
  };

  const handleUpdateUser = async (
    event: CustomEvent<{ username: string; password: string }>,
  ) => {
    showForm = false;
    await loadUsers();
  };

  const addUser = () => {
    editingUser = null;
    showForm = true;
  };
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Users</BreadcrumbItem>
</Breadcrumb>
<h2>{$_("Users")}</h2>

<br />

{#if !users}
  <div><Loading /></div>
{/if}

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
{#if users}
  <div style="padding-bottom: 10px;" class="content-right">
    <Button
      size="small"
      icon={AddAlt}
      on:click={() => {
        addUser();
      }}>{$_("Add User")}</Button
    >
  </div>
  {#if users.length === 0}
    <div>{$_("No users registered yet.")}</div>
  {/if}
  <DataTable
    headers={[
      {
        key: "user",
        value: $_("Name"),
      },
      {
        key: "dataconsumed",
        value: $_("Data Consumed"),
      },
      {
        key: "allowaccess",
        value: $_("Allow Access"),
      },
      {
        key: "actions",
        value: $_("Actions"),
      },
    ]}
    rows={users?.map((user) => {
      return {
        id: user.username,
        user: user.username,
        dataconsumed: user.dataconsumed,
        allowaccess: user.allowaccess,
      };
    })}
  >
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "actions"}
        <div style="float:right;">
          <Button
            size="small"
            icon={RowDelete}
            iconDescription={$_("Delete")}
            on:click={() => deleteUser(row.id)}>{$_("Delete")}</Button
          >
          <Button
            size="small"
            icon={Edit}
            iconDescription={$_("Edit User")}
            on:click={() => editUser(row.id)}>{$_("Edit User")}</Button
          >
          {#if row.allowaccess}
            <Button
              size="small"
              icon={Stop}
              iconDescription={$_("Disable Internet Access")}
              on:click={async () => {
                await $store.api.updateUser({
                  username: row.id,
                  password: "",
                  allowaccess: false,
                });
                loadUsers();
              }}>{$_("Disable Internet Access")}</Button
            >
          {:else}
            <Button
              size="small"
              icon={Restart}
              iconDescription={$_("Enable Internet Access")}
              on:click={async () => {
                await $store.api.updateUser({
                  username: row.id,
                  password: "",
                  allowaccess: true,
                });
                loadUsers();
              }}>{$_("Enable Internet Access")}</Button
            >
          {/if}
        </div>
      {:else if cell.key === "dataconsumed"}
        {bytesToSize(cell.value)}
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
{/if}
