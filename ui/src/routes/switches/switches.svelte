<script lang="ts">
  import { Column, Grid, Row, Toggle } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import ToggleComponent from "../../components/toggle.svelte";
</script>

<h1>Welcome</h1>
<p>
  This is a simple web proxy that can be used to block requests to certain
  domains. It can be used to block ads, tracking, and other unwanted content.
</p>
<p>
  To use it, you need to configure your browser to use this proxy. The proxy is
  running on port 8080 on this machine. You can configure your browser to use
  this proxy by following the instructions for your browser:
</p>
<br />
<br />
<ToggleComponent
  settingName="enable_dns_server"
  label="DNS Server"
  labelA="Not Running"
  labelB="Running"
/>
<br />
<!-- <ToggleComponent
  settingName="enable_https_filtering"
  label="Proxy Server"
  labelA="Not Running"
  labelB="Running"
/> -->
