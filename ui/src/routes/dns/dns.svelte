<script lang="ts">
  import Dnslists from "../../components/dnslists.svelte";
  import DnsArecords from "../../components/dnsArecords.svelte";
  import { Column, Grid, Row } from "carbon-components-svelte";
</script>

<h2>DNS Server</h2>
<br />
<DnsArecords />
<br />
<br />
<Dnslists />
