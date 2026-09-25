<script lang="ts">
  import { Button } from "carbon-components-svelte";
  import { gsNavigate } from "../../lib/navigate";
  import PageShell from "../../components/layout/PageShell.svelte";
  import ResourceState from "../../components/layout/ResourceState.svelte";
</script>

<PageShell
  title="Page not found"
  description="The requested GateSentry administration page does not exist."
  narrow
>
  <ResourceState
    state="empty"
    title="We could not find that page"
    message="Return to the overview or choose another destination from the navigation."
  >
    <Button on:click={() => gsNavigate("/")}>Return to overview</Button>
  </ResourceState>
</PageShell>
