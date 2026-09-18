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
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";

  let search = "";
  let actionFilter = "";
  let layerFilter = "";
  let interval: ReturnType<typeof setInterval> | null = null;
  let logsToRender: any[] = [];

  const buildQuery = () => {
    const params: string[] = [];
    if (search.length > 0) params.push("domain=" + encodeURIComponent(search));
    if (actionFilter) params.push("action=" + actionFilter);
    if (layerFilter) params.push("layer=" + layerFilter);
    return params.length > 0 ? "?" + params.join("&") : "";
  };

  const loadDecisions = () => {
    $store.api.doCall("/decisions" + buildQuery()).then((json: any) => {
      const items = (json && json.items) || [];
      logsToRender = items.map((item: any, index: number) => ({
        id: item.ip + item.time + index + item.url,
        time: format(item.time * 1000),
        ip: item.ip,
        url: _.truncate(item.url, { length: 50 }),
        action: item.action || "",
        layer: item.layer || "",
        reason: _.truncate(item.reason || item.matched_rule || "", {
          length: 40,
        }),
      }));
    });
  };

  const clearSearch = () => {
    search = "";
    loadDecisions();
  };

  const startInterval = () => {
    if (interval) clearInterval(interval);
    interval = setInterval(loadDecisions, 5000);
  };

  $: if (actionFilter || layerFilter) {
    loadDecisions();
    startInterval();
  }

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  onMount(() => {
    loadDecisions();
    startInterval();
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
    <div style="display: flex; gap: 16px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 15px;">
      <div style="flex: 1; min-width: 200px;">
        <Search bind:value={search} on:clear={clearSearch} placeholder="Search by domain..." />
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
        { key: "ip", value: "Client IP" },
        { key: "url", value: "URL" },
        { key: "action", value: "Action" },
        { key: "layer", value: "Layer" },
        { key: "reason", value: "Reason" },
      ]}
      rows={logsToRender}
    />
  </Column>
</Row>
