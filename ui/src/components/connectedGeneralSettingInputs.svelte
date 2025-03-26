<script lang="ts">
  export let keyName: string;
  export let labelText: string;
  export let title: string;
  export let helperText: string = "";
  export let type: string = "text";
  export let disabled: boolean = false;

  import { TextInput } from "carbon-components-svelte";
  import { store } from "../store/apistore";
  import { _ } from "svelte-i18n";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  import { onMount } from "svelte";

  const SETTING_GENERAL_SETTINGS = "general_settings";
  let data = null;
  let internalFormValue = null;

  export const updateDataOnBackend = () => {
    updateNetwork(internalFormValue);
  };

  const loadAPIData = async () => {
    try {
      const json = await $store.api.getSetting("general_settings");
      data = JSON.parse(json.Value);
    } catch (error) {
      console.error(
        "[GatesentryUI] Unable to load settings (possibly due to logout)",
      );
    }
  };

  const updateNetwork = async (updateData) => {
    const response = await $store.api.setSetting(
      SETTING_GENERAL_SETTINGS,
      JSON.stringify(updateData),
    );
    console.log("response", response);
    if (response === false) {
      notificationstore.add(
        createNotificationError({ subtitle: $_("Unable to save setting") }, $_),
      );
    } else {
      notificationstore.add(
        createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
      );
    }
    await loadAPIData();
  };

  const updateField = async (event) => {
    const value = event.target.value;
    if (value == data[keyName]) return;
    const updateData = {
      ...data,
      [keyName]: value,
    };

    try {
      const response = await $store.api.setSetting(
        SETTING_GENERAL_SETTINGS,
        JSON.stringify(updateData),
      );
      if (response) {
        notificationstore.add(
          createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
        );
      } else {
        notificationstore.add(
          createNotificationError(
            { subtitle: $_("Unable to save setting") },
            $_,
          ),
        );
      }
    } catch (error) {
      console.error("[GatesentryUI] Error updating setting", error);
    }
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
  value={(data && data[keyName]) ?? ""}
  on:blur={updateField}
/>
