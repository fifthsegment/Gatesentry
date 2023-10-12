<script lang="ts">
  import {
    Button,
    Form,
    RadioButton,
    RadioButtonGroup,
    TextInput,
  } from "carbon-components-svelte";
  import Modal from "../../components/modal.svelte";
  import { _ } from "svelte-i18n";
  import { createEventDispatcher, onDestroy, onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { notificationstore } from "../../store/notifications";
  import { createNotificationError } from "../../lib/utils";
  import type { UserType } from "../../types";
  const dispatch = createEventDispatcher();
  export let showForm = false;
  export let user: UserType | null = null;
  // new user
  let username = user?.username ?? "";
  let password = user?.password ?? "";
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

<Form on:submit={handleCreateUser}>
  <TextInput
    bind:value={username}
    id="user"
    labelText={$_("User")}
    disabled={user ? true : false}
  />
  <br />
  <TextInput
    bind:value={password}
    id="password"
    labelText={$_("Password")}
    type="password"
  />
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
