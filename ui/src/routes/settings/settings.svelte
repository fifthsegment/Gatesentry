<script lang="ts">
  import { Button, FormLabel, TextInput } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  let data = null;
  let enable_https_filtering = null;
  const loadAPIData = () => {
    const url = "/settings/general_settings";

    $store.api.doCall(url).then(function (json) {
      data = JSON.parse(json.Value);
    });

    $store.api.doCall("/settings/enable_https_filtering").then((json) => {
      enable_https_filtering = json.Value;
    });
  };

  const toggleHttpsFilteringStatus = () => {
    const url = "/settings/enable_https_filtering";
    var datatosend = {
      key: "enable_https_filtering",
      value: enable_https_filtering == "true" ? "false" : "true",
    };
    $store.api.doCall(url, "post", datatosend).then(function (json) {
      console.log("json", json);
      loadAPIData();
      // that.mapDataToState(json);
    });
  };

  loadAPIData();
</script>

<h2>Settings</h2>

<TextInput
  title={$_("Log Location")}
  labelText={$_("Log Location")}
  value={data?.log_location}
/>

<TextInput
helperText={$_("Leave blank to keep the current password")}
  type="password"
  title={$_("Password")}
  labelText={$_("Password")}
  value={data?.admin_password}
/>

<div style="margin-top: 15px;">
  <FormLabel>{$_("HTTPS Filtering")}</FormLabel>
  <div>
    {enable_https_filtering == "true" ? "Enabled" : "Disabled"}
  </div>
  <div style="margin-top: 5px;">
    <Button size="small" on:click={toggleHttpsFilteringStatus}>Toggle</Button>
  </div>
</div>
