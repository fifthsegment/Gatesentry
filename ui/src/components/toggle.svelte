<script lang="ts">
  import { Loading, Toggle } from "carbon-components-svelte";
  export let settingName = "enable_https_filtering";
  export let label = "Label"
  export let labelA = "On"
  export let labelB = "Off"
  import { store } from "../store/apistore";

  let settingValue = "";
  const loadAPIData = () => {
    $store.api.doCall("/settings/"+settingName).then((json) => {
        settingValue = json.Value;
    });
  };

  const toggleSettingStatus = () => {
    console.log("toggling")
    const url = "/settings/"+ settingName;
    var datatosend = {
      key: settingName,
      value: settingValue == "true" ? "false" : "true",
    };

    $store.api.doCall(url, "post", datatosend).then(function (json) {
      loadAPIData();
    });
  };

  loadAPIData();
</script>

<span>
    {#if settingValue == ""}
        <Loading />
    {:else}
        <Toggle 
            labelText="{label}"
            labelA="{labelA}"
            labelB="{labelB}" 
            toggled={settingValue == "true"}
            on:change={toggleSettingStatus}
        />    
    {/if}
</span>