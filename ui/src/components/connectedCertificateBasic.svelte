<script lang="ts">
  import { FormLabel, TextArea } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  let value = "";
  export let settingName;
  export let label;
  $: inputId = `certificate-${settingName}`;

  const loadAPIData = async () => {
    if (settingName === undefined) {
      return;
    }
    const json = await $store.api.getSetting(settingName);
    value = json.Value;
  };

  let onBlur = async (event) => {
    const value = event.target.value;
    const response = await $store.api.setSetting(settingName, value);
    if (response === false) {
      notificationstore.add(
        createNotificationError(
          { subtitle: $_("Unable to save certificate data") },
          $_,
        ),
      );
    } else {
      notificationstore.add(
        createNotificationSuccess(
          { subtitle: $_("Certificate data saved") },
          $_,
        ),
      );
    }
    loadAPIData();
  };

  $: {
    loadAPIData();
  }
</script>

<div class="certificate-field">
  <FormLabel for={inputId}>{label}</FormLabel>
  <TextArea id={inputId} {value} on:blur={onBlur}></TextArea>
</div>

<style>
  .certificate-field {
    margin-top: 1rem;
  }
</style>
