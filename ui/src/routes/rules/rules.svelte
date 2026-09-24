<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import Policygroups from "./policygroups.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Policies</BreadcrumbItem>
</Breadcrumb>
<h2>Policies</h2>
<p class="lead">
  Each device follows one policy. Pick what to block, turn on safe search or bedtime, and add devices to it.
</p>
<Policygroups />

<style>
  .lead {
    max-width: 48rem;
    margin: 0 0 1.5rem;
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
</style>
