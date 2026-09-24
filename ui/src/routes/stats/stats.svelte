<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    ContentSwitcher,
    DataTable,
    InlineNotification,
    Switch,
    Tag,
  } from "carbon-components-svelte";
  import "@carbon/charts/styles.css";
  import { AreaChart, ScaleTypes, Alignments } from "@carbon/charts";
  import { onDestroy, onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";

  type Count = { key: string; count: number };
  type Summary = {
    window: string;
    total: number;
    by_action: Record<string, number>;
    by_layer: Record<string, number>;
    top_blocked_domains: Count[];
    top_domains: Count[];
    top_reasons: Count[];
    affected_devices: { ip: string; device: string; count: number }[];
    blocks_by_policy: { id: string; name: string; count: number }[];
    inspection_failures: number;
    unique_clients: number;
    unique_domains: number;
    timeline: { start: number; total: number; blocked: number }[];
  };

  const windows = [1, 7];
  const layerNames: Record<string, string> = {
    dns: "DNS",
    explicit_proxy: "Proxy",
    transparent_proxy: "Transparent proxy",
    content: "Content inspection",
  };
  const actionNames: Record<string, string> = {
    allow: "Allowed",
    block: "Blocked",
    inspect: "Inspected",
    bypass: "Bypassed",
    error: "Errors",
    unknown: "Other",
  };

  let selected = 1;
  let summary: Summary | null = null;
  let loadError = "";
  let chart: any = null;
  let chartHolder: HTMLDivElement;
  let timer: ReturnType<typeof setInterval> | null = null;

  const fmt = (n: number | undefined) =>
    typeof n === "number" ? n.toLocaleString() : "—";
  const pct = (part: number, whole: number) =>
    whole > 0 ? ((part / whole) * 100).toFixed(1) + "%" : "—";

  const chartData = (s: Summary) =>
    (s.timeline || []).flatMap((b) => {
      const date = new Date(b.start * 1000).toISOString();
      return [
        { group: "Requests", date, value: b.total },
        { group: "Blocked", date, value: b.blocked },
      ];
    });

  const drawChart = (s: Summary) => {
    const data = chartData(s);
    if (!chartHolder) return;
    if (!chart) {
      chart = new AreaChart(chartHolder, {
        data,
        options: {
          axes: {
            bottom: { mapsTo: "date", scaleType: ScaleTypes.TIME },
            left: { mapsTo: "value", scaleType: ScaleTypes.LINEAR },
          },
          curve: "curveMonotoneX",
          height: "320px",
          toolbar: { enabled: false },
          legend: { alignment: Alignments.LEFT },
        },
      });
    } else {
      chart.model.setData(data);
    }
  };

  const load = async () => {
    try {
      const days = windows[selected];
      summary = await $store.api.doCall(`/decisions/summary?days=${days}`);
      loadError = "";
      if (summary) drawChart(summary);
    } catch (error) {
      loadError = "Unable to load statistics.";
    }
  };

  const countRows = (items: Count[] | undefined, prefix: string) =>
    (items || []).map((d, i) => ({ id: prefix + i, key: d.key, count: fmt(d.count) }));

  const mapRows = (m: Record<string, number> | undefined, names: Record<string, string>, total: number) =>
    Object.entries(m || {})
      .sort((a, b) => b[1] - a[1])
      .map(([k, v]) => ({ id: k, key: names[k] || k, count: fmt(v), share: pct(v, total) }));

  $: blocked = summary?.by_action?.block || 0;

  const selectWindow = (index: number) => {
    selected = index;
    load();
  };

  onMount(() => {
    load();
    timer = setInterval(load, 30000);
  });

  onDestroy(() => {
    if (timer) clearInterval(timer);
    if (chart) chart.destroy();
  });
</script>

<div class="page">
  <Breadcrumb noTrailingSlash>
    <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
    <BreadcrumbItem>{$_("Stats")}</BreadcrumbItem>
  </Breadcrumb>

  <header class="page-head">
    <h2>{$_("Stats")}</h2>
    <div class="switcher">
      <ContentSwitcher
        size="sm"
        selectedIndex={selected}
        on:change={(e) => selectWindow(e.detail)}
      >
        <Switch text={$_("24 hours")} />
        <Switch text={$_("7 days")} />
      </ContentSwitcher>
    </div>
  </header>

  {#if loadError}
    <InlineNotification kind="error" title={$_("Stats unavailable")} subtitle={loadError} hideCloseButton />
  {/if}

  <section class="tiles">
    <div class="tile">
      <span class="tile-label">{$_("Requests")}</span>
      <span class="tile-value">{fmt(summary?.total)}</span>
    </div>
    <div class="tile">
      <span class="tile-label">{$_("Blocked")}</span>
      <span class="tile-value">{fmt(blocked)}</span>
      <span class="tile-foot">{pct(blocked, summary?.total || 0)} {$_("of requests")}</span>
    </div>
    <div class="tile">
      <span class="tile-label">{$_("Active clients")}</span>
      <span class="tile-value">{fmt(summary?.unique_clients)}</span>
    </div>
    <div class="tile">
      <span class="tile-label">{$_("Distinct domains")}</span>
      <span class="tile-value">{fmt(summary?.unique_domains)}</span>
    </div>
    <div class="tile">
      <span class="tile-label">{$_("Inspection errors")}</span>
      <span class="tile-value">{fmt(summary?.inspection_failures)}</span>
      {#if summary && summary.inspection_failures > 0}
        <Tag type="red" size="sm">{$_("Check the logs")}</Tag>
      {/if}
    </div>
  </section>

  <section class="panel">
    <h3>{$_("Requests over time")}</h3>
    <div bind:this={chartHolder} class="chart"></div>
  </section>

  {#if summary}
    <div class="columns">
      <section class="panel">
        <h3>{$_("Most blocked domains")}</h3>
        {#if summary.top_blocked_domains?.length}
          <DataTable
            size="short"
            headers={[{ key: "key", value: $_("Domain") }, { key: "count", value: $_("Blocks") }]}
            rows={countRows(summary.top_blocked_domains, "b")}
          />
        {:else}
          <p class="helper">{$_("Nothing blocked in this window.")}</p>
        {/if}
      </section>
      <section class="panel">
        <h3>{$_("Most requested domains")}</h3>
        {#if summary.top_domains?.length}
          <DataTable
            size="short"
            headers={[{ key: "key", value: $_("Domain") }, { key: "count", value: $_("Requests") }]}
            rows={countRows(summary.top_domains, "t")}
          />
        {:else}
          <p class="helper">{$_("No requests in this window.")}</p>
        {/if}
      </section>
      <section class="panel">
        <h3>{$_("Most active devices")}</h3>
        {#if summary.affected_devices?.length}
          <DataTable
            size="short"
            headers={[
              { key: "device", value: $_("Device") },
              { key: "count", value: $_("Requests") },
            ]}
            rows={summary.affected_devices.map((d, i) => ({
              id: "d" + i,
              device: d.device || d.ip,
              count: fmt(d.count),
            }))}
          />
        {:else}
          <p class="helper">{$_("No device activity in this window.")}</p>
        {/if}
      </section>
      <section class="panel">
        <h3>{$_("Blocks by policy")}</h3>
        {#if summary.blocks_by_policy?.length}
          <DataTable
            size="short"
            headers={[{ key: "name", value: $_("Policy") }, { key: "count", value: $_("Blocks") }]}
            rows={summary.blocks_by_policy.map((p) => ({ id: p.id, name: p.name, count: fmt(p.count) }))}
          />
        {:else}
          <p class="helper">{$_("No policy blocks in this window.")}</p>
        {/if}
      </section>
      <section class="panel">
        <h3>{$_("Block reasons")}</h3>
        {#if summary.top_reasons?.length}
          <DataTable
            size="short"
            headers={[{ key: "key", value: $_("Reason") }, { key: "count", value: $_("Decisions") }]}
            rows={countRows(summary.top_reasons, "r")}
          />
        {:else}
          <p class="helper">{$_("No recorded reasons in this window.")}</p>
        {/if}
      </section>
      <section class="panel">
        <h3>{$_("Traffic breakdown")}</h3>
        <DataTable
          size="short"
          headers={[
            { key: "key", value: $_("Layer") },
            { key: "count", value: $_("Requests") },
            { key: "share", value: $_("Share") },
          ]}
          rows={mapRows(summary.by_layer, layerNames, summary.total)}
        />
        <DataTable
          size="short"
          headers={[
            { key: "key", value: $_("Outcome") },
            { key: "count", value: $_("Requests") },
            { key: "share", value: $_("Share") },
          ]}
          rows={mapRows(summary.by_action, actionNames, summary.total)}
        />
      </section>
    </div>
  {/if}
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    max-width: 80rem;
  }
  .page-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 1rem;
  }
  .switcher {
    min-width: 18rem;
  }
  .helper {
    margin: 0;
    color: var(--cds-text-helper, #6f6f6f);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 1rem;
  }
  .tile,
  .panel {
    background: var(--cds-ui-01, #f4f4f4);
    padding: 1rem 1.25rem;
    min-width: 0;
  }
  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.35rem;
  }
  .tile-label {
    font-size: 0.875rem;
    color: var(--cds-text-secondary, #525252);
  }
  .tile-value {
    font-size: 2rem;
    font-weight: 300;
    line-height: 1.2;
  }
  .tile-foot {
    font-size: 0.75rem;
    color: var(--cds-text-helper, #6f6f6f);
  }
  .panel h3 {
    margin-bottom: 0.75rem;
  }
  .chart {
    min-height: 320px;
  }
  .columns {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr));
    gap: 1rem;
  }
  @media (max-width: 480px) {
    .columns {
      grid-template-columns: 1fr;
    }
    .switcher {
      min-width: 0;
      width: 100%;
    }
  }
  .panel :global(.bx--data-table-container) {
    min-width: 0;
    overflow-x: auto;
  }
  .panel :global(.bx--data-table-container + .bx--data-table-container) {
    margin-top: 1rem;
  }
</style>
