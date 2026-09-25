<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import ToggleComponent from "../../components/toggle.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import SectionPanel from "../../components/layout/SectionPanel.svelte";
</script>

<PageShell
  title="Services"
  description="Control the gateway services running on this installation."
  narrow
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Services</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <SectionPanel
    title="DNS server"
    description="Devices configured to use GateSentry DNS depend on this service for domain filtering."
  >
    <ToggleComponent
      settingName="enable_dns_server"
      label="DNS Server"
      labelA="Not running"
      labelB="Running"
    />
  </SectionPanel>
</PageShell>
