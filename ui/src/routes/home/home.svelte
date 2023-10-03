<script lang="ts">
  import { afterUpdate } from "svelte";
  import { store } from "../../store/apistore";
  import { navigate } from "svelte-routing";
  let loggedIn = false;
  $: loggedIn = $store.api.loggedIn;

  afterUpdate(() => {
    if (loggedIn) {
      navigate("/stats");
    }
  });
</script>
