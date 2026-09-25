<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    DataTable,
    InlineLoading,
    InlineNotification,
    OverflowMenu,
    OverflowMenuItem,
    Tag,
  } from "carbon-components-svelte";
  import { AddAlt, Renew } from "carbon-icons-svelte";
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import Modal from "../../components/modal.svelte";
  import { bytesToSize } from "../../lib/utils";
  import { store } from "../../store/apistore";
  import type { UserType } from "../../types";
  import Usermodal from "./usermodal.svelte";

  const enforcementOptions = [
    { value: "true", label: $_("Require a proxy user") },
    { value: "false", label: $_("Do not require a proxy user") },
  ];

  let users: Array<UserType> = [];
  let loading = true;
  let error = "";
  let success = "";
  let showForm = false;
  let savingUser = false;
  let userForm: Usermodal | null = null;
  let editingUser: UserType | null = null;
  let pendingDelete: UserType | null = null;
  let deleting = false;

  const headers = [
    { key: "user", value: $_("Username") },
    { key: "dataconsumed", value: $_("Data consumed") },
    { key: "allowaccess", value: $_("Access") },
    { key: "actions", value: $_("Actions") },
  ];

  $: rows = users.map((user) => ({
    id: user.username,
    user: user.username,
    dataconsumed: user.dataconsumed ?? 0,
    allowaccess: user.allowaccess,
  }));

  async function loadUsers() {
    loading = true;
    error = "";
    try {
      const response = await $store.api.getUsers();
      users = Array.isArray(response?.users) ? response.users : [];
    } catch (caught) {
      error =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("Proxy users could not be loaded.");
    } finally {
      loading = false;
    }
  }

  function closeForm() {
    showForm = false;
    editingUser = null;
    savingUser = false;
  }

  function addUser() {
    editingUser = null;
    showForm = true;
  }

  function editUser(username: string) {
    editingUser = users.find((user) => user.username === username) ?? null;
    showForm = true;
  }

  async function userSaved(event: CustomEvent<{ editing: boolean }>) {
    closeForm();
    success = event.detail.editing
      ? $_("Proxy user updated.")
      : $_("Proxy user created.");
    await loadUsers();
  }

  async function setAccess(username: string, allowaccess: boolean) {
    error = "";
    success = "";
    try {
      const response = await $store.api.updateUser({
        username,
        password: "",
        allowaccess,
      });
      if (!response?.ok) {
        throw new Error(response?.error ?? response?.Error ?? $_("Access could not be updated."));
      }
      success = allowaccess
        ? $_("Internet access enabled.")
        : $_("Internet access paused.");
      await loadUsers();
    } catch (caught) {
      error =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("Access could not be updated.");
    }
  }

  function requestDelete(username: string) {
    pendingDelete = users.find((user) => user.username === username) ?? null;
  }

  async function confirmDelete() {
    if (!pendingDelete || deleting) return;

    deleting = true;
    error = "";
    success = "";
    const username = pendingDelete.username;
    try {
      const response = await $store.api.deleteUser(username);
      if (response?.ok === false) {
        throw new Error(response?.error ?? response?.Error ?? $_("The proxy user could not be deleted."));
      }
      pendingDelete = null;
      success = $_("Proxy user deleted.");
      await loadUsers();
    } catch (caught) {
      error =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("The proxy user could not be deleted.");
    } finally {
      deleting = false;
    }
  }

  onMount(loadUsers);
</script>

<PageShell
  title={$_("Proxy users")}
  description={$_(
    "Proxy users are sign-in credentials for devices and browsers that connect through the GateSentry proxy. They are separate from administrator accounts used to manage this dashboard.",
  )}
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("Proxy users")}</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>
  <svelte:fragment slot="actions">
    <Button size="small" icon={AddAlt} on:click={addUser}>
      {$_("Add proxy user")}
    </Button>
  </svelte:fragment>

  {#if error}
    <InlineNotification
      kind="error"
      title={$_("Proxy users unavailable")}
      subtitle={error}
      on:close={() => (error = "")}
    />
  {/if}
  {#if success}
    <InlineNotification
      kind="success"
      title={$_("Proxy users updated")}
      subtitle={success}
      on:close={() => (success = "")}
    />
  {/if}

  <section class="panel enforcement" aria-labelledby="proxy-enforcement-title">
    <div>
      <h3 id="proxy-enforcement-title">{$_("Proxy authentication")}</h3>
      <p>
        {$_(
          "When required, the proxy challenges every new connection for one of the credentials below. A user's access setting can then allow or pause internet use without deleting the credential.",
        )}
      </p>
    </div>
    <ConnectedSettingInput
      keyName="EnableUsers"
      type="radio"
      title={$_("Require proxy sign-in")}
      labelText={$_("Require proxy sign-in")}
      helperText={$_("This applies to proxy traffic only; DNS clients do not send proxy credentials.")}
      orientation="vertical"
      radioOptions={enforcementOptions}
    />
  </section>

  <section class="users-panel" aria-labelledby="proxy-users-list-title">
    <div class="section-head">
      <div>
        <h3 id="proxy-users-list-title">{$_("Credentials")}</h3>
        <p>{$_("Manage proxy sign-in and each user's immediate internet access.")}</p>
      </div>
      <Button
        kind="ghost"
        size="small"
        icon={Renew}
        disabled={loading}
        on:click={loadUsers}
      >
        {$_("Refresh")}
      </Button>
    </div>

    {#if loading && users.length === 0}
      <div class="state-panel">
        <InlineLoading description={$_("Loading proxy users…")} />
      </div>
    {:else if users.length === 0}
      <div class="state-panel empty-state">
        <h4>{$_("No proxy users yet")}</h4>
        <p>
          {$_(
            "Add a credential before requiring proxy sign-in so clients are not locked out.",
          )}
        </p>
        <Button size="small" icon={AddAlt} on:click={addUser}>
          {$_("Add proxy user")}
        </Button>
      </div>
    {:else}
      <DataTable sortable size="compact" {headers} {rows}>
        <svelte:fragment slot="cell" let:row let:cell>
          {#if cell.key === "actions"}
            <OverflowMenu flipped iconDescription={$_("User actions")}>
              <OverflowMenuItem
                text={$_("Edit")}
                on:click={() => editUser(row.id)}
              />
              <OverflowMenuItem
                text={row.allowaccess
                  ? $_("Pause internet access")
                  : $_("Allow internet access")}
                on:click={() => setAccess(row.id, !row.allowaccess)}
              />
              <OverflowMenuItem
                danger
                text={$_("Delete")}
                on:click={() => requestDelete(row.id)}
              />
            </OverflowMenu>
          {:else if cell.key === "dataconsumed"}
            {bytesToSize(Number(cell.value) || 0)}
          {:else if cell.key === "allowaccess"}
            <Tag type={cell.value ? "green" : "warm-gray"} size="sm">
              {cell.value ? $_("Allowed") : $_("Paused")}
            </Tag>
          {:else}
            {cell.value}
          {/if}
        </svelte:fragment>
      </DataTable>
    {/if}
  </section>
</PageShell>

<Modal
  bind:open={showForm}
  title={editingUser ? $_("Edit proxy user") : $_("Add proxy user")}
  label={$_("Proxy access")}
  primaryButtonText={savingUser
    ? $_("Saving…")
    : editingUser
    ? $_("Save changes")
    : $_("Create user")}
  secondaryButtonText={$_("Cancel")}
  primaryButtonDisabled={savingUser}
  shouldSubmitOnEnter
  hasForm
  preventCloseOnClickOutside
  on:close={closeForm}
  on:submit={() => userForm?.submit()}
>
  <Usermodal
    bind:this={userForm}
    bind:saving={savingUser}
    user={editingUser}
    on:saved={userSaved}
  />
</Modal>

<Modal
  open={Boolean(pendingDelete)}
  title={$_("Delete proxy user?")}
  label={$_("Confirm deletion")}
  primaryButtonText={deleting ? $_("Deleting…") : $_("Delete user")}
  secondaryButtonText={$_("Cancel")}
  primaryButtonDisabled={deleting}
  danger
  size="xs"
  preventCloseOnClickOutside
  on:close={() => {
    if (!deleting) pendingDelete = null;
  }}
  on:submit={confirmDelete}
>
  <p>
    {$_("Delete proxy user")}
    <strong>{pendingDelete?.username ?? ""}</strong>?
    {$_("Their current proxy credentials will stop working.")}
  </p>
</Modal>

<style>
  .section-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .section-head p,
  .enforcement p,
  .empty-state p {
    max-width: 48rem;
    margin: 0.5rem 0 0;
    color: var(--cds-text-secondary, #525252);
    line-height: 1.45;
  }

  .panel,
  .users-panel {
    background: var(--cds-ui-01, #f4f4f4);
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .enforcement {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(18rem, 28rem);
    gap: 2rem;
    padding: 1.5rem;
  }

  .users-panel {
    min-width: 0;
  }

  .section-head {
    padding: 1.25rem 1.5rem;
  }

  .state-panel {
    min-height: 8rem;
    display: flex;
    align-items: center;
    padding: 1.5rem;
    border-top: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .empty-state {
    align-items: flex-start;
    flex-direction: column;
    gap: 1rem;
  }

  .empty-state p {
    margin-top: 0;
  }

  @media (max-width: 42rem) {
    .enforcement {
      grid-template-columns: 1fr;
      gap: 1.25rem;
    }
  }
</style>
