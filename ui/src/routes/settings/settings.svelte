<script lang="ts">
  import { _ } from "svelte-i18n";

  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
  import ConnectedCertificateComposed from "../../components/connectedCertificateComposed.svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Settings</BreadcrumbItem>
</Breadcrumb>

<h2>Settings</h2>

<div class="settings-container">
  <ConnectedGeneralSettingInput
    keyName="log_location"
    title={$_("Log Location")}
    labelText={$_("Log Location")}
    type="text"
    helperText=""
  />

  <ConnectedGeneralSettingInput
    keyName="admin_username"
    helperText=""
    type="text"
    title={$_("Admin username")}
    labelText={$_("Admin username")}
    disabled={true}
  />

  <ConnectedGeneralSettingInput
    keyName="timezone"
    title={$_("Timezone")}
    labelText={$_("Timezone")}
    type="text"
    helperText={$_("Enter the timezone (e.g., UTC, GMT+2)")}
  />

  <HttpsToggle />

  <ConnectedCertificateComposed />
</div>

<style>
  .settings-container {
    display: flex;
    flex-direction: column;
    gap: 1rem; /* Add spacing between components */
  }
</style>
