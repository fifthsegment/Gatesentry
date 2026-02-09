<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    Row,
  } from "carbon-components-svelte";
  import DeviceList from "./devicelist.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Devices</BreadcrumbItem>
</Breadcrumb>
<h2>Network Devices</h2>
<br />

<Row>
  <Column>
    <DeviceList />
  </Column>
</Row>
