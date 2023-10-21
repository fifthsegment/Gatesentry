<script lang="ts">
  import { Breadcrumb, BreadcrumbItem } from "carbon-components-svelte";
  import Rulelist from "./rulelist.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Rules</BreadcrumbItem>
</Breadcrumb>
<h2>Rules</h2>
<Rulelist />
