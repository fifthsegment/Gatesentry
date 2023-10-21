<script lang="ts">
  export let type;
  import { _ } from "svelte-i18n";

  import Filtereditor from "../../components/filtereditor.svelte";

  // bVxTPTOXiqGRbhF
</script>

{#if type == "blockedfiletypes"}
  <Filtereditor
    filterId="JHGJiwjkGOeglsk"
    title={$_("Blocked Content Types")}
    showColumns={["content", "actions"]}
    description={$_(
      "Add MIME types to block here. For example to block .jpg files add image/jpeg, to block .webp files add image/webp.",
    )}
  />
{:else if type == "blockedkeywords"}
  <Filtereditor
    filterId="bVxTPTOXiqGRbhF"
    title={$_("Blocked Keywords")}
    description={$_(
      "Add/Update things to block here. The score is used to determine how bad the keyword is. The higher the score, the worse the keyword.",
    )}
  />
{:else if type == "blockedurls"}
  <Filtereditor
    filterId="bTXmTXgTuXpJuOZ"
    title={$_("Block List")}
    showColumns={["content", "actions"]}
    description={$_("Add Sites to block here.")}
  />
{:else if type == "excludeurls"}
  <Filtereditor
    filterId="JHGJiwjkGOeglsd"
    title={$_("Excluded URLs")}
    showColumns={["content", "actions"]}
    description={$_("Add URLs to exclude from filtering here.")}
  />
{:else if type == "excludehosts"}
  <Filtereditor
    filterId="CeBqssmRbqXzbHR"
    title={$_("Excluded Hosts")}
    showColumns={["content", "actions"]}
    description={$_(
      "Add hostnames you want to exclude from filtering here. If SSL Bumping is enabled, these Hostnames will not be bumped.",
    )}
  />
{/if}
