<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    InlineLoading,
    InlineNotification,
  } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import Filtereditor from "../../components/filtereditor.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import SectionPanel from "../../components/layout/SectionPanel.svelte";
  import { gsNavigate } from "../../lib/navigate";
  import { store } from "../../store/apistore";

  type InspectionType =
    | "blockedfiletypes"
    | "blockedkeywords"
    | "excludeurls"
    | "excludehosts";

  type RouteConfiguration = {
    path: string;
    label: string;
    title: string;
    description: string;
    sectionTitle: string;
    sectionDescription: string;
    filterId: string;
    showColumns?: string[];
    warnsWhenDisabled: boolean;
  };

  export let type: InspectionType;

  const routes: Record<InspectionType, RouteConfiguration> = {
    blockedkeywords: {
      path: "/blockedkeywords",
      label: $_("Blocked keywords"),
      title: $_("Blocked keywords"),
      description: $_("Score words found in inspected HTTP and HTTPS response content."),
      sectionTitle: $_("Keyword scores"),
      sectionDescription: $_(
        "GateSentry totals matching scores and blocks the response when the filtering strictness threshold is reached.",
      ),
      filterId: "bVxTPTOXiqGRbhF",
      warnsWhenDisabled: true,
    },
    blockedfiletypes: {
      path: "/blockedfiletypes",
      label: $_("Blocked content types"),
      title: $_("Blocked content types"),
      description: $_("Block inspected proxy responses by their MIME content type."),
      sectionTitle: $_("Content types"),
      sectionDescription: $_(
        "Entries apply to every device using the proxy. Use a policy rule when a content type should apply only to selected devices.",
      ),
      filterId: "JHGJiwjkGOeglsk",
      showColumns: ["content", "actions"],
      warnsWhenDisabled: true,
    },
    excludehosts: {
      path: "/excludehosts",
      label: $_("Sites not inspected"),
      title: $_("Sites not inspected"),
      description: $_("Pass selected HTTPS hosts through without decrypting their traffic."),
      sectionTitle: $_("Host exclusions"),
      sectionDescription: $_(
        "Add apps or sites that fail under inspection, such as certificate-pinning services. Policies can still block these hosts by domain.",
      ),
      filterId: "CeBqssmRbqXzbHR",
      showColumns: ["content", "actions"],
      warnsWhenDisabled: false,
    },
    excludeurls: {
      path: "/excludeurls",
      label: $_("URLs not inspected"),
      title: $_("URLs not inspected"),
      description: $_("Exclude selected request URLs from content filtering after the connection is inspected."),
      sectionTitle: $_("URL exclusions"),
      sectionDescription: $_(
        "Use narrowly scoped URLs for content that should bypass response filtering. Use Sites not inspected when the TLS connection itself must not be decrypted.",
      ),
      filterId: "JHGJiwjkGOeglsd",
      showColumns: ["content", "actions"],
      warnsWhenDisabled: true,
    },
  };

  $: configuration = routes[type] ?? routes.blockedkeywords;

  let inspectionOn: boolean | null = null;
  let inspectionError = "";

  const loadInspectionState = async () => {
    inspectionError = "";
    try {
      const json = await $store.api.doCall("/settings/enable_https_filtering");
      inspectionOn = json?.Value === "true";
    } catch (caught) {
      inspectionOn = null;
      inspectionError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("HTTPS inspection status could not be checked.");
    }
  };

  onMount(loadInspectionState);
</script>

<PageShell title={configuration.title} description={configuration.description}>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("HTTPS inspection")}</BreadcrumbItem>
      <BreadcrumbItem>{configuration.label}</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <nav class="inspection-navigation" aria-label={$_("HTTPS inspection sections")}>
    {#each Object.values(routes) as route}
      <Button
        kind={route.path === configuration.path ? "primary" : "ghost"}
        size="small"
        on:click={() => gsNavigate(route.path)}
      >
        {route.label}
      </Button>
    {/each}
  </nav>

  {#if configuration.warnsWhenDisabled && inspectionOn === null && !inspectionError}
    <InlineLoading description={$_("Checking HTTPS inspection…")} />
  {:else if configuration.warnsWhenDisabled && inspectionOn === false}
    <InlineNotification
      kind="warning"
      lowContrast
      hideCloseButton
      title={$_("HTTPS inspection is off")}
      subtitle={$_(
        "This list applies to plain HTTP traffic only until HTTPS inspection is enabled in Settings and the GateSentry certificate is installed on each device. Use Policies to block whole domains.",
      )}
    />
  {:else if inspectionError}
    <InlineNotification
      kind="info"
      lowContrast
      title={$_("Inspection status unavailable")}
      subtitle={inspectionError}
      on:close={() => (inspectionError = "")}
    />
  {/if}

  {#if type === "blockedkeywords"}
    <SectionPanel
      title={$_("Filtering strictness")}
      description={$_(
        "Set the total keyword score at which GateSentry blocks an inspected page. For example, two matches scored 1,000 reach a threshold of 2,000.",
      )}
    >
      <div class="strictness-control">
        <ConnectedSettingInput
          keyName="strictness"
          title={$_("Filtering strictness")}
          labelText={$_("Filtering strictness")}
          type="number"
          helperText={$_("Lower thresholds block pages after fewer or lower-scored matches.")}
        />
      </div>
    </SectionPanel>
  {/if}

  <SectionPanel
    title={configuration.sectionTitle}
    description={configuration.sectionDescription}
  >
    <Filtereditor
      filterId={configuration.filterId}
      showColumns={configuration.showColumns ?? ["content", "score", "actions"]}
    />
  </SectionPanel>
</PageShell>

<style>
  .inspection-navigation {
    display: flex;
    max-width: 100%;
    flex-wrap: wrap;
    gap: 1px;
    padding: 1px;
    background: var(--cds-border-subtle, #c6c6c6);
  }

  .inspection-navigation :global(.bx--btn) {
    flex: 1 1 12rem;
    justify-content: center;
  }

  .strictness-control {
    max-width: 24rem;
  }
</style>
