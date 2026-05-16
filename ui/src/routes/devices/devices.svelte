<script lang="ts">
  import DeviceList from "./devicelist.svelte";
  import { Devices as DevicesIcon } from "carbon-icons-svelte";
</script>

<div class="gs-page-title">
  <DevicesIcon size={24} />
  <h2>Network Devices</h2>
</div>

<p class="dev-desc">
  Devices discovered on your network via DNS queries, mDNS, and DDNS updates.
</p>

<DeviceList />

<style>
  .dev-desc {
    font-size: 0.875rem;
    color: #525252;
    margin: 0 0 0.75rem 0;
  }
</style>
