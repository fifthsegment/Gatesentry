<script lang="ts">
  export let keyName: string;
  export let labelText: string;
  export let title: string;
  export let helperText;
  export let type;
  export let disabled = false;
  export let disableOnblur = false;

  import { TextInput } from "carbon-components-svelte";
  import { store } from "../store/apistore";
  import { _ } from "svelte-i18n";
  import { notificationstore } from "../store/notifications";
  import {
    buildNotificationError,
    buildNotificationSuccess,
  } from "../lib/utils";
  import { onDestroy, onMount } from "svelte";

  let data = null;
  let internalFormValue = null;

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
        buildNotificationError({ subtitle: $_("Unable to save setting") }, $_),
      );
    } else {
      notificationstore.add(
        buildNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
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

  onMount(() => {
    loadAPIData();
  });
</script>

<TextInput
  {title}
  {labelText}
  {helperText}
  {type}
  {disabled}
  value={data ?? ""}
  on:blur={updateField}
/>
