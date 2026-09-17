<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import Policygroups from "./policygroups.svelte";
  import Rulelist from "./rulelist.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Rules</BreadcrumbItem>
</Breadcrumb>
<h2>Rules</h2>
<Policygroups />
<h3 class="advanced-heading">Advanced rule editor</h3>
<Rulelist />

<style>
  .advanced-heading {
    margin-top: 2rem;
  }
</style>
