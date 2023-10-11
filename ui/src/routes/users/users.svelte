<script lang="ts">
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { Button, DataTable, Loading } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { AddAlt, RowDelete } from "carbon-icons-svelte";

  type User = {
    user: string;
    dataconsumed: string;
    allowaccess: boolean;
  };

  let users: Array<User> | null = null;

  const loadUsers = async () => {
    users = null;
    users = (await $store.api.getUsers()).users;
  };

  onMount(loadUsers);

  const addUser = async () => {
    const user = prompt($_("Enter the name of the user to add"));
    if (user) {
      users = [...users, { user, dataconsumed: "0", allowaccess: true }];
      await $store.api.updateUsers(users);
      await loadUsers();
    }
  };

  const deleteUser = async (user: string) => {
    if (confirm($_("Are you sure you want to delete this user?"))) {
      users = users.filter((u) => u.user !== user);
      await $store.api.updateUsers(users);
      await loadUsers();
    }
  };
</script>

<h3>{$_("Registered Users")}</h3>

<br />

{#if !users}
  <div><Loading /></div>
{/if}

{#if users}
  <div style="display: flex; justify-content: flex-end; padding-bottom: 10px;">
    <Button size="small" icon={AddAlt} on:click={addUser}>
      {$_("Add User")}
    </Button>
  </div>

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
        id: user.user,
        user: user.user,
        dataconsumed: user.dataconsumed,
        allowaccess: user.allowaccess,
      };
    })}
  >
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "actions"}
        <div style="float:right;">
          <Button
            icon={RowDelete}
            iconDescription={$_("Delete")}
            on:click={() => deleteUser(row.id)}
          />
        </div>
      {:else}
        {cell.value}
      {/if}
    </svelte:fragment>
  </DataTable>
{/if}
