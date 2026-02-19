<script lang="ts">
  import {
    Button,
    Form,
    RadioButton,
    RadioButtonGroup,
    TextInput,
  } from "carbon-components-svelte";
  import { View, ViewOff } from "carbon-icons-svelte";
  import Modal from "../../components/modal.svelte";
  import { _ } from "svelte-i18n";
  import { createEventDispatcher, onDestroy, onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { notificationstore } from "../../store/notifications";
  import { createNotificationError } from "../../lib/utils";
  import type { UserType } from "../../types";
  const dispatch = createEventDispatcher();
  export let user: UserType | null = null;
  // new user
  let username = user?.username ?? "";
  let password = user?.password ?? "";
  let showPassword = false;
  let allowAccess =
    user?.allowaccess && user.allowaccess == true ? "true" : "false";

  onDestroy(() => {
    username = "";
    password = "";
    allowAccess = "true";
  });

  const showError = (message: string) => {
    notificationstore.add(
      createNotificationError(
        {
          title: $_("Error"),
          subtitle: $_("Error : " + message),
        },
        $_,
      ),
    );
  };

  const handleCreateUser = async (event) => {
    event.preventDefault();
    try {
      if (user) {
        const response = await $store.api.updateUser({
          username,
          password,
          allowaccess: allowAccess == "true" ? true : false,
        });
        if (response.ok) {
          dispatch("updateuser", { username, password });
        } else {
          showError(response.error);
        }
        return;
      } else {
        const response = await $store.api.createUser({
          username,
          password,
          allowaccess: allowAccess == "true" ? true : false,
        });
        if (response.ok) {
          dispatch("createuser", { username, password });
        } else {
          showError(response.error);
        }
      }
    } catch (error) {
      showError(error.message);
    }
  };
</script>

<Form on:submit={handleCreateUser} autocomplete="off">
  <TextInput
    bind:value={username}
    id="field1"
    name="field1"
    autocomplete="off"
    labelText={$_("Proxy Account Name")}
    disabled={user ? true : false}
  />
  <br />
  <div class="credential-field" class:credential-masked={!showPassword}>
    <TextInput
      bind:value={password}
      id="field2"
      name="field2"
      autocomplete="off"
      labelText={$_("Proxy Credential")}
    />
    <!-- svelte-ignore a11y-no-static-element-interactions a11y-click-events-have-key-events -->
    <span
      class="credential-toggle"
      on:click={() => (showPassword = !showPassword)}
      title={showPassword ? $_("Hide") : $_("Show")}
    >
      {#if showPassword}
        <ViewOff size={18} />
      {:else}
        <View size={18} />
      {/if}
    </span>
  </div>
  <br />
  <RadioButtonGroup
    legendText="Allow internet access"
    bind:selected={allowAccess}
  >
    <RadioButton value="true" labelText={$_("Allow access")} />
    <RadioButton value="false" labelText={$_("Deny access")} />
  </RadioButtonGroup>
  <div class="content-right" style="margin-top: 10px;">
    {#if user}
      <Button type="submit" on:click={handleCreateUser}
        >{$_("Save User")}</Button
      >
    {:else}
      <Button type="submit" on:click={handleCreateUser}
        >{$_("Create User")}</Button
      >
    {/if}
  </div>
</Form>

<style>
  .credential-field {
    position: relative;
  }
  .credential-masked :global(input) {
    -webkit-text-security: disc;
    font-family: text-security-disc, monospace;
    letter-spacing: 0.125em;
  }
  .credential-toggle {
    position: absolute;
    right: 1px;
    bottom: 1px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 38px;
    cursor: pointer;
    color: #525252;
    transition: color 0.12s;
  }
  .credential-toggle:hover {
    color: #161616;
  }
</style>
