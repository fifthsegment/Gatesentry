<script lang="ts">
  import { _ } from "svelte-i18n";

  import Filtereditor from "../../components/filtereditor.svelte";
  import { Filter as FilterIcon } from "carbon-icons-svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
</script>

<div class="gs-page-title">
  <FilterIcon size={24} />
  <h2>{$_("Blocked Keywords")}</h2>
</div>
<p class="fe-desc">
  {$_(
    "Add/Update keywords to block here. The score is used to determine how bad the keyword is. The higher the score, the worse the keyword.",
  )}
</p>

<div class="fe-setting">
  <ConnectedSettingInput
    keyName="strictness"
    title={$_("Filtering strictness")}
    labelText={$_("Filtering strictness")}
    type="number"
    helperText=""
  />
  <p class="fe-helper">
    {$_(
      "Filtering strictness is the threshold beyond which you want the page to be blocked",
    )}
  </p>
  <p class="fe-helper">
    {$_(
      "Example for a value of 2000, if 2 keywords with a score of 1000 are found, the page will be blocked. If 4 keywords with a score of 500 are found, the page will be blocked. If 1 keyword with a score of 2000 is found, the page will be blocked.",
    )}
  </p>
</div>

<Filtereditor filterId="bVxTPTOXiqGRbhF" />

<style>
  .fe-desc {
    font-size: 0.875rem;
    color: #525252;
    margin-bottom: 16px;
  }
  .fe-setting {
    margin-bottom: 16px;
  }
  .fe-helper {
    font-size: 0.75rem;
    color: #6f6f6f;
    margin-top: 4px;
  }
</style>
