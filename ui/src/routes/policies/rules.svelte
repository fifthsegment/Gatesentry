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
  A policy decides what a device can reach: categories and domains it blocks, sites it always allows, safe search,
  and rules for bedtime, school hours, or parts of a site. DNS enforces it for every device that uses GateSentry for
  DNS; the proxy also enforces URL and response-type rules.
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
