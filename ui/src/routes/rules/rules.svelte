<script lang="ts">
  import Rulelist from "./rulelist.svelte";
  import Ruledetail from "./rform.svelte";
  import { Rule } from "carbon-icons-svelte";

  let view: "list" | "detail" = "list";
  let selectedRule = null;
  let isNew = false;

  function openRule(e) {
    selectedRule = e.detail;
    isNew = false;
    view = "detail";
  }

  function createRule(e) {
    selectedRule = e.detail;
    isNew = true;
    view = "detail";
  }

  function goBack() {
    view = "list";
    selectedRule = null;
  }
</script>

{#if view === "list"}
  <div class="gs-page-title">
    <Rule size={24} />
    <h2>Proxy Filter Rules</h2>
  </div>
  <Rulelist on:open={openRule} on:create={createRule} />
{:else}
  <Ruledetail rule={selectedRule} {isNew} on:back={goBack} />
{/if}
