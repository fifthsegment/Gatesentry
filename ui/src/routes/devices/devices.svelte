<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import DeviceList from "./devicelist.svelte";
</script>

<PageShell
  title="Network devices"
  description="Review devices GateSentry has observed, assign protection, and manage explicit identity links."
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Devices</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <DeviceList />
</PageShell>
