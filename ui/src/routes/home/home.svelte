<script lang="ts">
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  let loggedIn = false;
  $: loggedIn = $store.api.loggedIn;

  // afterUpdate(() => {
  //   if (loggedIn) {
  //     navigate("/stats");
  //   }
  // });
</script>

<h2>{$_("Welcome to Gatesentry")}</h2>
<br />
<div>
  {$_("Click on the menu to the left to get started.")}
</div>
