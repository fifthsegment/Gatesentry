<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    DataTable,
    Search,
    Select,
    SelectItem,
  } from "carbon-components-svelte";
  import { format } from "timeago.js";
  import { store } from "../../store/apistore";
  import { buildDeviceNameMap, formatDeviceAddress } from "../../lib/devicenames";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import ResourceState from "../../components/layout/ResourceState.svelte";
  import SectionPanel from "../../components/layout/SectionPanel.svelte";

  let search = "";
  let actionFilter = "";
  let layerFilter = "";
  let decisions: any[] = [];
  let deviceNames = new Map<string, string>();
  let decisionTimer: ReturnType<typeof setTimeout> | null = null;
  let deviceTimer: ReturnType<typeof setTimeout> | null = null;
  let decisionsLoading = false;
  let decisionRefreshPending = false;
  let devicesLoading = false;
  let mounted = false;
  let destroyed = false;
  let activeFilter = "";
  let initialLoaded = false;
  let loadError = "";

  const buildQuery = () => {
    const params: string[] = [];
    if (search.length > 0) params.push("domain=" + encodeURIComponent(search));
    if (actionFilter) params.push("action=" + actionFilter);
    if (layerFilter) params.push("layer=" + layerFilter);
    return params.length > 0 ? "?" + params.join("&") : "";
  };

  const loadDecisions = async () => {
    if (decisionsLoading || destroyed) return;
    decisionsLoading = true;
    const query = buildQuery();
    try {
      const json = await $store.api.doCall("/decisions" + query);
      if (!destroyed && query === buildQuery()) {
        decisions = json?.items || [];
        loadError = "";
        initialLoaded = true;
      }
    } catch {
      if (!destroyed && !initialLoaded) loadError = "Unable to load decisions.";
    } finally {
      decisionsLoading = false;
      if (destroyed) return;
      if (decisionRefreshPending) {
        decisionRefreshPending = false;
        loadDecisions();
      } else {
        decisionTimer = setTimeout(loadDecisions, 5000);
      }
    }
  };

  const loadDevices = async () => {
    if (devicesLoading || destroyed) return;
    devicesLoading = true;
    try {
      const json = await $store.api.doCall("/devices");
      if (!destroyed) deviceNames = buildDeviceNameMap(json?.devices);
    } catch {
      // Device discovery can be unavailable; raw decision IPs remain useful.
    } finally {
      devicesLoading = false;
      if (!destroyed) deviceTimer = setTimeout(loadDevices, 60000);
    }
  };

  const refreshDecisions = () => {
    if (decisionTimer) clearTimeout(decisionTimer);
    decisionTimer = null;
    if (decisionsLoading) {
      decisionRefreshPending = true;
      return;
    }
    loadDecisions();
  };

  const clearSearch = () => {
    search = "";
    refreshDecisions();
  };

  $: logsToRender = decisions.map((item: any, index: number) => ({
    id: item.ip + item.time + index + item.url,
    time: format(item.time * 1000),
    client: formatDeviceAddress(deviceNames, item.ip),
    url: _.truncate(item.url, { length: 50 }),
    action: item.action || "",
    layer: item.layer || "",
    reason: _.truncate(item.reason || item.matched_rule || "", { length: 40 }),
  }));

  $: {
    const nextFilter = actionFilter + "\0" + layerFilter;
    if (mounted && nextFilter !== activeFilter) {
      activeFilter = nextFilter;
      refreshDecisions();
    }
  }

  onMount(() => {
    mounted = true;
    activeFilter = actionFilter + "\0" + layerFilter;
    loadDecisions();
    loadDevices();
  });

  onDestroy(() => {
    destroyed = true;
    if (decisionTimer) clearTimeout(decisionTimer);
    if (deviceTimer) clearTimeout(deviceTimer);
  });
</script>

<PageShell
  title="Decision log"
  description="Structured filtering decisions from DNS, proxy, and content inspection layers."
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Logs</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>

  <SectionPanel title="Filter decisions" description="Search recent activity or narrow it by outcome and enforcement layer.">
    <div class="filters">
      <div class="filters__search">
        <Search bind:value={search} on:clear={clearSearch} placeholder="Search by domain" labelText="Search decisions" />
      </div>
      <Select bind:selected={actionFilter} labelText="Action">
        <SelectItem value="" text="All actions" />
        <SelectItem value="block" text="Block" />
        <SelectItem value="allow" text="Allow" />
        <SelectItem value="bypass" text="Bypass" />
        <SelectItem value="inspect" text="Inspect" />
        <SelectItem value="error" text="Error" />
      </Select>
      <Select bind:selected={layerFilter} labelText="Layer">
        <SelectItem value="" text="All layers" />
        <SelectItem value="dns" text="DNS" />
        <SelectItem value="explicit_proxy" text="Explicit proxy" />
        <SelectItem value="transparent_proxy" text="Transparent proxy" />
        <SelectItem value="content" text="Content" />
      </Select>
    </div>

    {#if loadError}
      <ResourceState state="error" title="Decisions unavailable" message={loadError} onRetry={refreshDecisions} />
    {:else if !initialLoaded}
      <ResourceState state="loading" message="Loading filtering decisions" />
    {:else if logsToRender.length === 0}
      <ResourceState
        state="empty"
        title="No decisions found"
        message="Try clearing the search or choosing a different filter."
      />
    {:else}
      <DataTable
        sortable
        size="medium"
        headers={[
          { key: "time", value: "Time" },
          { key: "client", value: "Client" },
          { key: "url", value: "URL" },
          { key: "action", value: "Action" },
          { key: "layer", value: "Layer" },
          { key: "reason", value: "Reason" },
        ]}
        rows={logsToRender}
      />
    {/if}
  </SectionPanel>
</PageShell>

<style>
  .filters {
    display: grid;
    grid-template-columns: minmax(16rem, 1fr) repeat(2, minmax(10rem, 13rem));
    align-items: end;
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .filters__search {
    min-width: 0;
  }
  @media (max-width: 48rem) {
    .filters {
      grid-template-columns: 1fr;
    }
  }
</style>
