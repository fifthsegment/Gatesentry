<script lang="ts">
  import { Breadcrumb, BreadcrumbItem, Toggle } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import ToggleComponent from "../../components/toggle.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Toggle Services</BreadcrumbItem>
</Breadcrumb>
<h2>Toggle Services</h2>

<br />

<ToggleComponent
  settingName="enable_dns_server"
  label="DNS Server"
  labelA="Not Running"
  labelB="Running"
/>
