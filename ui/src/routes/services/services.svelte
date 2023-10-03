<script lang="ts">
  import { Toggle } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import ToggleComponent from "../../components/toggle.svelte";
</script>

<h3>Toggle Services</h3>

<br />

<ToggleComponent
  settingName="enable_dns_server"
  label="DNS Server"
  labelA="Not Running"
  labelB="Running"
/>
