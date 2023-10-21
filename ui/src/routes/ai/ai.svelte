<script lang="ts">
  import { _ } from "svelte-i18n";

  import Toggle from "../../components/toggle.svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";

  let enable_ai_image_filtering = "";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Gatesentry AI</BreadcrumbItem>
</Breadcrumb>
<h2>Gatesentry AI</h2>
<br />
<div>
  This is an experimental feature. It is not recommended to use this in
  production. To use this feature an AI service is required, the instructions to
  download the service would soon be available.
</div>
<br />
<div>
  Gatesentry AI is a feature that allows you to use the power of AI to detect
  and block images that are not safe for work.
</div>
<br />
<ConnectedSettingInput
  keyName="ai_scanner_url"
  title={$_("AI Service URL")}
  labelText={$_("AI Service URL")}
  type="text"
  helperText=""
/>
<br />

<Toggle
  bind:settingValue={enable_ai_image_filtering}
  settingName="enable_ai_image_filtering"
  label={$_("Enable AI Image Filtering")}
  labelA={$_("Disabled")}
  labelB={$_("Enabled")}
  noNotification={true}
/>

<br />
