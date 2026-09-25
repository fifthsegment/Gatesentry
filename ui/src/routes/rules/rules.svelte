<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import Policygroups from "./policygroups.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Policies</BreadcrumbItem>
</Breadcrumb>
<div class="page-heading">
  <h2>Policies</h2>
  <p class="lead">
    Choose protection for each device, test a decision, or adjust the
    gateway-wide baseline.
  </p>
</div>
<Policygroups />

<style>
  .page-heading {
    margin-bottom: 1.5rem;
  }
  .page-heading h2 {
    margin-bottom: 0.5rem;
  }
  .lead {
    max-width: 48rem;
    margin: 0;
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
</style>
