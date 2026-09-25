<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import Policygroups from "./policygroups.svelte";
</script>

<PageShell
  title="Policies"
  description="Choose protection for each device, test a decision, or adjust the gateway-wide baseline."
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Policies</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <Policygroups />
</PageShell>
