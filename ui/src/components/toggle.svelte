<script lang="ts">
  import { Loading, Toggle } from "carbon-components-svelte";
  export let settingName = "enable_https_filtering";
  export let label = "Label";
  export let labelA = "On";
  export let labelB = "Off";
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { onMount } from "svelte";
  import { notificationstore } from "../store/notifications";
  import { buildNotificationSuccess } from "../lib/utils";

  export let settingValue = "";
  export let preClickEvent = null;
  export let hide = false;
  export let noNotification = false;
  const loadAPIData = () => {
    $store.api.doCall("/settings/" + settingName).then((json) => {
      settingValue = json.Value;
    });
  };

  const toggleSettingStatus = async () => {
    if (preClickEvent) {
      await preClickEvent();
    }
    const url = "/settings/" + settingName;
    var datatosend = {
      key: settingName,
      value: settingValue == "true" ? "false" : "true",
    };

    $store.api.doCall(url, "post", datatosend).then(function (json) {
      loadAPIData();
      if (noNotification) {
        return;
      }
      notificationstore.add(
        buildNotificationSuccess(
          {
            title: $_("Success"),
            subtitle: $_("Setting updated"),
          },
          $_,
        ),
      );
    });
  };

  onMount(() => {
    loadAPIData();
  });

  $: {
    loadAPIData();
  }
</script>

<span>
  {#if hide == true}{:else if settingValue == ""}
    <Loading />
  {:else}
    <Toggle
      labelText={label}
      {labelA}
      {labelB}
      toggled={settingValue == "true"}
      on:change={toggleSettingStatus}
    />
  {/if}
</span>
