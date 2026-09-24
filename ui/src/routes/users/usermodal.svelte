<script lang="ts">
  import {
    InlineNotification,
    RadioButton,
    RadioButtonGroup,
    TextInput,
  } from "carbon-components-svelte";
  import { createEventDispatcher } from "svelte";
  import { _ } from "svelte-i18n";

  import { store } from "../../store/apistore";
  import type { UserType } from "../../types";

  const dispatch = createEventDispatcher<{
    saved: { username: string; editing: boolean };
    createuser: { username: string };
    updateuser: { username: string };
  }>();

  export let user: UserType | null = null;
  export let saving = false;

  let formUser: UserType | null | undefined;
  let username = "";
  let password = "";
  let allowAccess = "false";
  let attempted = false;
  let error = "";

  $: if (user !== formUser) {
    formUser = user;
    username = user?.username ?? "";
    password = "";
    allowAccess = user?.allowaccess ? "true" : "false";
    attempted = false;
    error = "";
  }

  $: usernameInvalid = attempted && username.trim().length < 3;
  $: passwordInvalid =
    attempted && (!user || password.length > 0) && password.length < 10;
  $: valid =
    username.trim().length >= 3 &&
    (user ? password.length === 0 || password.length >= 10 : password.length >= 10);

  function responseError(response: any) {
    return response?.error ?? response?.Error ?? $_("The user could not be saved.");
  }

  export async function submit() {
    attempted = true;
    error = "";
    if (!valid || saving) return;

    saving = true;
    const editing = Boolean(user);
    const normalizedUsername = user?.username ?? username.trim();

    try {
      const payload = {
        username: normalizedUsername,
        password,
        allowaccess: allowAccess === "true",
      };
      const response = editing
        ? await $store.api.updateUser(payload)
        : await $store.api.createUser(payload);

      if (!response?.ok) {
        error = responseError(response);
        return;
      }

      const detail = { username: normalizedUsername, editing };
      dispatch("saved", detail);
      dispatch(editing ? "updateuser" : "createuser", {
        username: normalizedUsername,
      });
    } catch (caught) {
      error =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("The user could not be saved.");
    } finally {
      saving = false;
    }
  }
</script>

<div class="user-form">
  {#if error}
    <InlineNotification
      lowContrast
      kind="error"
      title={$_("User not saved")}
      subtitle={error}
      on:close={() => (error = "")}
    />
  {/if}

  <TextInput
    bind:value={username}
    id="proxy-user-name"
    labelText={$_("Username")}
    helperText={user
      ? $_("Usernames cannot be changed.")
      : $_("At least 3 characters.")}
    disabled={Boolean(user) || saving}
    invalid={usernameInvalid}
    invalidText={$_("Enter a username with at least 3 characters.")}
    data-modal-primary-focus
  />

  <TextInput
    bind:value={password}
    id="proxy-user-password"
    labelText={user ? $_("New password") : $_("Password")}
    helperText={user
      ? $_("Leave blank to keep the current password. New passwords need at least 10 characters.")
      : $_("At least 10 characters.")}
    type="password"
    disabled={saving}
    invalid={passwordInvalid}
    invalidText={$_("Enter a password with at least 10 characters.")}
  />

  <RadioButtonGroup
    legendText={$_("Internet access")}
    orientation="vertical"
    disabled={saving}
    bind:selected={allowAccess}
  >
    <RadioButton
      id="proxy-user-access-allow"
      value="true"
      labelText={$_("Allow access")}
    />
    <RadioButton
      id="proxy-user-access-deny"
      value="false"
      labelText={$_("Deny access")}
    />
  </RadioButtonGroup>
</div>

<style>
  .user-form {
    display: grid;
    gap: 1rem;
  }
</style>
