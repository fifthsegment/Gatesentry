<script lang="ts">
  export let keyName: string;
  export let labelText: string;
  export let title: string;
  export let helperText;
  export let type;
  export let disabled = false;
  export let disableOnblur = false;

  import {
    RadioButtonGroup,
    TextInput,
    PasswordInput,
    RadioButton,
  } from "carbon-components-svelte";
  import { store } from "../store/apistore";
  import { _ } from "svelte-i18n";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  import { onDestroy, onMount } from "svelte";

  let data = null;
  let internalFormValue = null;
  let loaded = false;

  export const updateDataOnBackend = () => {
    updateNetwork(internalFormValue);
  };

  const loadAPIData = async () => {
    try {
      const json = await $store.api.getSetting(keyName);
      data = json.Value;
    } catch (error) {
      console.error(
        "[GatesentryUI] Unable to load settings (possibly due to logout)",
      );
    }
  };

  const updateNetwork = async (keyValue) => {
    const response = await $store.api.setSetting(keyName, keyValue);
    if (response === false) {
      notificationstore.add(
        createNotificationError({ subtitle: $_("Unable to save setting") }, $_),
      );
    } else {
      notificationstore.add(
        createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
      );
    }
  };

  const updateField = async (event) => {
    const value = event.target.value;
    if (value == data) return;

    internalFormValue = value;
    if (disableOnblur) return;
    updateNetwork(internalFormValue);
  };

  const updateFieldRadio = async (event) => {
    internalFormValue = data;
    if (disableOnblur) return;
    updateNetwork(internalFormValue);
  };

  onMount(async () => {
    await loadAPIData();
    loaded = true;
  });

  onDestroy(() => {
    data = null;
    internalFormValue = null;
    loaded = false;
  });
</script>

{#if loaded}
  {#if type == "radio"}
    <RadioButtonGroup
      legendText={labelText}
      bind:selected={data}
      on:change={updateFieldRadio}
    >
      <RadioButton value="true" labelText={$_("True")} />
      <RadioButton value="false" labelText={$_("False")} />
    </RadioButtonGroup>
  {:else if type == "password"}
    <PasswordInput
      {labelText}
      {helperText}
      {disabled}
      tooltipPosition="left"
      value={data ?? ""}
      on:blur={updateField}
    />
  {:else}
    <TextInput
      {title}
      {labelText}
      {helperText}
      {type}
      {disabled}
      value={data ?? ""}
      on:blur={updateField}
    />
  {/if}
{/if}
