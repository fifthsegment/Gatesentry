<script lang="ts">
  export let type;
  import { _ } from "svelte-i18n";

  import Filtereditor from "../../components/filtereditor.svelte";
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    Row,
    Grid,
  } from "carbon-components-svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";

  // bVxTPTOXiqGRbhF
</script>

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("Filters")}</BreadcrumbItem>
    </Breadcrumb>
  </Column>
</Row>

{#if type == "blockedfiletypes"}
  <h2>{$_("Blocked Content Types")}</h2>
  <div class="description">
    {$_(
      "Add MIME types to block here. For example to block .jpg files add image/jpeg, to block .webp files add image/webp.",
    )}
  </div>
  <Filtereditor
    filterId="JHGJiwjkGOeglsk"
    showColumns={["content", "actions"]}
  />
{:else if type == "blockedkeywords"}
  <h2>{$_("Blocked Keywords")}</h2>
  <div class="description">
    {$_(
      "Add/Update keywords to block here. The score is used to determine how bad the keyword is. The higher the score, the worse the keyword.",
    )}
  </div>
  <div>
    <Row>
      <Column md={4} lg={4}
        ><ConnectedSettingInput
          keyName="strictness"
          title={$_("Filtering strictness")}
          labelText={$_("Filtering strictness")}
          type="number"
          helperText=""
        />
        <label class="bx--label"
          >{$_(
            "Filtering strictness is the threshold beyond which you want the page to be blocked",
          )}</label
        >
        <label class="bx--label"
          >{$_(
            "Example for a value of 2000, if 2 keywords with a score of 1000 are found, the page will be blocked. If 4 keywords with a score of 500 are found, the page will be blocked. If 1 keyword with a score of 2000 is found, the page will be blocked.",
          )}</label
        >
        <br />
      </Column>
      <Column md={12} lg={12}>
        <Filtereditor filterId="bVxTPTOXiqGRbhF" />
      </Column>
    </Row>
  </div>
{:else if type == "blockedurls"}
  <h2>{$_("Block List")}</h2>
  <div class="description">
    {$_("Add Sites to block here.")}
  </div>
  <Filtereditor
    filterId="bTXmTXgTuXpJuOZ"
    showColumns={["content", "actions"]}
  />
{:else if type == "excludeurls"}
  <h2>{$_("Excluded URLs")}</h2>
  <div class="description">
    {$_("Add URLs to exclude from filtering here.")}
  </div>
  <Filtereditor
    filterId="JHGJiwjkGOeglsd"
    showColumns={["content", "actions"]}
  />
{:else if type == "excludehosts"}
  <h2>{$_("Excluded Hosts")}</h2>
  <div class="description">
    {$_(
      "Add hostnames you want to exclude from filtering here. If SSL Bumping is enabled, these Hostnames will not be bumped.",
    )}
  </div>
  <Filtereditor
    filterId="CeBqssmRbqXzbHR"
    showColumns={["content", "actions"]}
  />
{/if}

<style>
  .description {
    margin: 20px 0px;
  }
</style>
