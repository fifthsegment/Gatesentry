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
  import { InlineNotification } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";

  // These lists act on decrypted HTTPS traffic, so say plainly when
  // inspection is off rather than let them look like they are working.
  let inspectionOn: boolean | null = null;
  onMount(async () => {
    try {
      const json = await $store.api.doCall("/settings/enable_https_filtering");
      inspectionOn = json?.Value === "true";
    } catch {
      inspectionOn = null;
    }
  });
</script>

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("HTTPS inspection")}</BreadcrumbItem>
    </Breadcrumb>
  </Column>
</Row>

{#if type !== "excludehosts" && inspectionOn === false}
  <InlineNotification
    kind="warning"
    lowContrast
    hideCloseButton
    title="HTTPS inspection is off."
    subtitle="This list only applies to plain HTTP traffic until you turn on HTTPS filtering in Settings and install the GateSentry certificate on each device. To block whole sites, use Policies."
  />
{/if}

{#if type == "blockedfiletypes"}
  <h2>{$_("Blocked content types")}</h2>
  <div class="description">
    {$_(
      "Responses of these types are blocked on every device that uses the proxy. For example image/jpeg blocks .jpg files and video/ blocks all video. To block a type for one policy only, add it to a rule in Policies.",
    )}
  </div>
  <Filtereditor
    filterId="JHGJiwjkGOeglsk"
    showColumns={["content", "actions"]}
  />
{:else if type == "blockedkeywords"}
  <h2>{$_("Blocked keywords")}</h2>
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
  <h2>{$_("Sites not inspected")}</h2>
  <div class="description">
    {$_(
      "HTTPS traffic to these hosts is passed through without decryption. Add apps and sites that break when inspected, such as banking apps or apps that pin their certificates. Policies still block these sites by domain.",
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
