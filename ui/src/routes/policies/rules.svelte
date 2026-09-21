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
  A policy decides what a device can reach: pick the categories and domains,
  choose block or allow, then assign the devices it covers. Every other device
  keeps the gateway default policy.
</p>
<p class="lead">
  The DNS server enforces a policy by domain. A rule that turns on TLS
  inspection also reaches the proxy, which is what lets it test URL paths and
  response content types.
</p>
<Policygroups />

<style>
  .lead {
    max-width: 48rem;
    margin: 0 0 0.5rem;
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
</style>
