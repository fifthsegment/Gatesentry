<script lang="ts">
  import { _ } from "svelte-i18n";
  import Certificate from "../../components/connectedCertificate.svelte";

  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
</script>

<h2>Settings</h2>

<br />
<ConnectedGeneralSettingInput
  keyName="log_location"
  title={$_("Log Location")}
  labelText={$_("Log Location")}
  type="text"
  helperText=""
/>
<br />
<ConnectedGeneralSettingInput
  keyName="admin_username"
  helperText={""}
  type="text"
  title={$_("Admin username")}
  labelText={$_("Admin username")}
  disabled={true}
/>

<HttpsToggle />

<Certificate settingName="capem" label={$_("HTTPS Filtering - Certificate")} />

<Certificate settingName="keypem" label={$_("HTTPS Filtering - Key")} />
