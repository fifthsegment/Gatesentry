<script lang="ts">
  import { FormLabel, TextArea } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";
  import {
    buildNotificationError,
    buildNotificationSuccess,
  } from "../lib/utils";
  let value = "";
  export let settingName;
  export let label;

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
        buildNotificationError(
          { subtitle: $_("Unable to save certificate data") },
          $_,
        ),
      );
    } else {
      notificationstore.add(
        buildNotificationSuccess(
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

<br />
<FormLabel>{label}</FormLabel>

<TextArea {value} on:blur={onBlur}></TextArea>
