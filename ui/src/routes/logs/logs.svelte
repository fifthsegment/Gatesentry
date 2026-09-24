<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Row,
    Search,
    Select,
    SelectItem,
    Tag,
  } from "carbon-components-svelte";
  import { format } from "timeago.js";
  import { store } from "../../store/apistore";
  import {
    buildDeviceNameMap,
    formatDeviceAddress,
  } from "../../lib/devicenames";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";

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
      if (!destroyed && query === buildQuery()) decisions = json?.items || [];
    } catch {
      // Keep the last successful page during a transient refresh failure.
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

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Logs</BreadcrumbItem>
    </Breadcrumb>
    <h2>Decision log</h2>
  </Column>
</Row>
<Row>
  <Column>
    <div style="margin: 20px 0px;">
      Structured filtering decisions from DNS and proxy layers.
    </div>
    <div style="margin-bottom: 15px;">
      <Tag>
        IMPORTANT: If you are using GateSentry on a Raspberry Pi please make
        sure to change GateSentry's log file location to RAM. You can do that by
        going to Settings and changing the log file location to "/tmp/log.db".
      </Tag>
    </div>
    <div
      style="display: flex; gap: 16px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 15px;"
    >
      <div style="flex: 1; min-width: 200px;">
        <Search
          bind:value={search}
          on:clear={clearSearch}
          placeholder="Search by domain..."
        />
      </div>
      <div style="min-width: 160px;">
        <Select bind:selected={actionFilter} labelText="Action">
          <SelectItem value="" text="All actions" />
          <SelectItem value="block" text="Block" />
          <SelectItem value="allow" text="Allow" />
          <SelectItem value="bypass" text="Bypass" />
          <SelectItem value="inspect" text="Inspect" />
          <SelectItem value="error" text="Error" />
        </Select>
      </div>
      <div style="min-width: 160px;">
        <Select bind:selected={layerFilter} labelText="Layer">
          <SelectItem value="" text="All layers" />
          <SelectItem value="dns" text="DNS" />
          <SelectItem value="explicit_proxy" text="Explicit proxy" />
          <SelectItem value="transparent_proxy" text="Transparent proxy" />
          <SelectItem value="content" text="Content" />
        </Select>
      </div>
    </div>
    <DataTable
      sortable
      size="medium"
      style="width:100%; min-height: 600px;"
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
  </Column>
</Row>
