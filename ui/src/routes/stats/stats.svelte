<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Dropdown,
    Loading,
    Row,
    Tag,
  } from "carbon-components-svelte";
  import "@carbon/charts/styles.css";
  import { AreaChart } from "@carbon/charts";
  import { onDestroy, onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";

  // ---------- Types ----------

  type HostData = { host: string; count: number };
  type BucketData = { total: number; hosts: HostData[] };
  type Keys = "blocked" | "all";
  type ResponseData = { [key in Keys]: { [date: string]: BucketData } };

  // ---------- State ----------

  /** Time-scale options for the chart x-axis */
  const scaleOptions = [
    { id: "7d", text: "Past 7 days" },
    { id: "24h", text: "Past 24 hours" },
    { id: "1h", text: "Past hour" },
  ];
  let selectedScale = "7d";

  let chart: any = null;
  let chartHolder: HTMLElement;

  /** Historical data from /stats/byUrl (fetched once on mount). */
  let historicalData: ResponseData | null = null;

  /**
   * Real-time counters accumulated from SSE "request" events.
   * Outer key = time-bucket label, inner key = domain, value = count.
   */
  let realtimeAll: Map<string, Map<string, number>> = new Map();
  let realtimeBlocked: Map<string, Map<string, number>> = new Map();

  let eventSource: EventSource | null = null;
  let connected = false;
  let eventsReceived = 0;

  // ---------- Helpers ----------

  /** Return a bucket key for a given timestamp + scale. */
  function bucketKey(ts: number, scale: string): string {
    const d = new Date(ts);
    if (scale === "1h") {
      // Truncate to the minute → "HH:MM"
      return (
        d.getHours().toString().padStart(2, "0") +
        ":" +
        d.getMinutes().toString().padStart(2, "0")
      );
    }
    if (scale === "24h") {
      // Truncate to the hour → ISO-like "YYYY-MM-DDTHH"
      return d.toISOString().slice(0, 13);
    }
    // 7d → "YYYY-MM-DD" (matches historical API keys)
    return d.toISOString().slice(0, 10);
  }

  /** Parse a bucket key back into a Date for the chart axis. */
  function bucketToDate(key: string, scale: string): Date {
    if (scale === "1h") {
      // key is "HH:MM" → use today's date
      const [h, m] = key.split(":").map(Number);
      const d = new Date();
      d.setHours(h, m, 0, 0);
      return d;
    }
    if (scale === "24h") {
      // key is "YYYY-MM-DDTHH"
      return new Date(key + ":00:00");
    }
    // 7d → "YYYY-MM-DD"
    return new Date(key + "T12:00:00");
  }

  /**
   * Merge historical + real-time data and produce chart data + top-5 tables.
   */
  function buildView(
    hist: ResponseData | null,
    rtAll: Map<string, Map<string, number>>,
    rtBlocked: Map<string, Map<string, number>>,
    scale: string,
  ) {
    const seriesAll = new Map<string, number>();
    const seriesBlocked = new Map<string, number>();
    const allCounts = new Map<string, number>();
    const blockedCounts = new Map<string, number>();

    // 1. Historical data (only used in 7-day view — the API returns daily buckets)
    if (hist && scale === "7d") {
      if (hist.all) {
        for (const [dateKey, bucket] of Object.entries(hist.all)) {
          seriesAll.set(dateKey, (seriesAll.get(dateKey) || 0) + bucket.total);
          for (const h of bucket.hosts)
            allCounts.set(h.host, (allCounts.get(h.host) || 0) + h.count);
        }
      }
      if (hist.blocked) {
        for (const [dateKey, bucket] of Object.entries(hist.blocked)) {
          seriesBlocked.set(
            dateKey,
            (seriesBlocked.get(dateKey) || 0) + bucket.total,
          );
          for (const h of bucket.hosts)
            blockedCounts.set(
              h.host,
              (blockedCounts.get(h.host) || 0) + h.count,
            );
        }
      }
    }

    // 2. Real-time SSE events (all scales)
    for (const [bucket, domains] of rtAll) {
      let total = 0;
      for (const [domain, count] of domains) {
        total += count;
        allCounts.set(domain, (allCounts.get(domain) || 0) + count);
      }
      seriesAll.set(bucket, (seriesAll.get(bucket) || 0) + total);
    }
    for (const [bucket, domains] of rtBlocked) {
      let total = 0;
      for (const [domain, count] of domains) {
        total += count;
        blockedCounts.set(domain, (blockedCounts.get(domain) || 0) + count);
      }
      seriesBlocked.set(bucket, (seriesBlocked.get(bucket) || 0) + total);
    }

    // 3. Build chart array, sorted by bucket key
    const allBuckets = new Set([...seriesAll.keys(), ...seriesBlocked.keys()]);
    const sorted = [...allBuckets].sort();
    const chartData: { group: string; date: Date; value: number }[] = [];

    for (const b of sorted) {
      const d = bucketToDate(b, scale);
      if (seriesAll.has(b))
        chartData.push({ group: "All Requests", date: d, value: seriesAll.get(b)! });
      if (seriesBlocked.has(b))
        chartData.push({ group: "Blocked Requests", date: d, value: seriesBlocked.get(b)! });
    }

    // 4. Top 5 tables
    const topAll = [...allCounts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([host, count], i) => ({ id: `all-${i}`, host, count }));

    const topBlocked = [...blockedCounts.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([host, count], i) => ({ id: `blocked-${i}`, host, count }));

    return { chartData, topAll, topBlocked };
  }

  // ---------- Reactive rendering ----------

  let chartData: { group: string; date: Date; value: number }[] = [];
  let topAllRows: { id: string; host: string; count: number }[] = [];
  let topBlockedRows: { id: string; host: string; count: number }[] = [];

  function refresh() {
    const result = buildView(
      historicalData,
      realtimeAll,
      realtimeBlocked,
      selectedScale,
    );
    chartData = result.chartData;
    topAllRows = result.topAll;
    topBlockedRows = result.topBlocked;
    if (chart) chart.model.setData(chartData);
  }

  // Throttle to ≤ 2 UI updates/second even at high QPS
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleRefresh() {
    if (refreshTimer) return;
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      refresh();
    }, 500);
  }

  // ---------- SSE ----------

  function connectSSE() {
    const jwt = localStorage.getItem("jwt") || "";
    const basePath = (window as any).__GS_BASE_PATH__ || "";
    const base = basePath === "/" ? "" : basePath;
    const url = `${base}/api/dns/events?token=${encodeURIComponent(jwt)}`;

    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      connected = true;
    };

    eventSource.onerror = () => {
      connected = false;
      // EventSource auto-reconnects on its own.
    };

    eventSource.onmessage = (msg) => {
      try {
        const evt = JSON.parse(msg.data);
        if (evt.type !== "request") return;

        eventsReceived++;

        const bucket = bucketKey(evt.ts, selectedScale);
        const domain: string = evt.domain || "unknown";

        // All requests
        if (!realtimeAll.has(bucket)) realtimeAll.set(bucket, new Map());
        const ab = realtimeAll.get(bucket)!;
        ab.set(domain, (ab.get(domain) || 0) + 1);

        // Blocked requests
        if (evt.blocked) {
          if (!realtimeBlocked.has(bucket))
            realtimeBlocked.set(bucket, new Map());
          const bb = realtimeBlocked.get(bucket)!;
          bb.set(domain, (bb.get(domain) || 0) + 1);
        }

        scheduleRefresh();
      } catch (e) {
        console.warn("[SSE] parse error:", e);
      }
    };
  }

  function disconnectSSE() {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
      connected = false;
    }
  }

  // ---------- Chart options (locale-aware) ----------

  function makeChartOptions(scale: string) {
    const locale = navigator.language || "en-US";

    return {
      title: "DNS requests",
      axes: {
        bottom: {
          title:
            scale === "7d"
              ? "Past 7 days"
              : scale === "24h"
                ? "Past 24 hours"
                : "Past hour",
          mapsTo: "date",
          scaleType: "time",
          ticks: {
            formatter: (d: Date) => {
              if (!(d instanceof Date) || isNaN(d.getTime())) return "";
              if (scale === "1h") {
                return d.toLocaleTimeString(locale, {
                  hour: "2-digit",
                  minute: "2-digit",
                });
              }
              if (scale === "24h") {
                return d.toLocaleTimeString(locale, {
                  hour: "2-digit",
                  minute: "2-digit",
                });
              }
              return d.toLocaleDateString(locale, {
                weekday: "short",
                month: "short",
                day: "numeric",
              });
            },
          },
        },
        left: {
          title: "Requests",
          mapsTo: "value",
          scaleType: "linear",
        },
      },
      height: "400px",
      toolbar: { enabled: false },
      color: {
        scale: {
          "All Requests": "#4589ff",
          "Blocked Requests": "#da1e28",
        },
      },
      legend: { alignment: "center" },
      points: { radius: 3 },
      curve: "curveMonotoneX",
    };
  }

  // When the scale dropdown changes, reset real-time buckets & rebuild
  function onScaleChange(e: CustomEvent) {
    selectedScale = e.detail.selectedId;
    realtimeAll = new Map();
    realtimeBlocked = new Map();
    eventsReceived = 0;

    if (chart) {
      chart.destroy();
      chart = null;
    }
    if (chartHolder) {
      // @ts-ignore
      chart = new AreaChart(chartHolder, {
        data: [],
        // @ts-ignore
        options: makeChartOptions(selectedScale),
      });
    }
    refresh();
  }

  // ---------- Lifecycle ----------

  onMount(async () => {
    chartHolder = document.getElementById("statschart") as HTMLElement;
    if (!chartHolder) throw new Error("Could not find chart holder element");

    // Create chart immediately (empty)
    // @ts-ignore
    chart = new AreaChart(chartHolder, {
      data: [],
      // @ts-ignore
      options: makeChartOptions(selectedScale),
    });

    // 1. One-shot fetch of historical 7-day data (no more polling)
    try {
      const json = (await $store.api.doCall("/stats/byUrl")) as ResponseData;
      if (json) {
        historicalData = json;
        refresh();
      }
    } catch (err) {
      console.error("Error fetching historical stats:", err);
    }

    // 2. Open SSE stream for real-time events
    connectSSE();
  });

  onDestroy(() => {
    disconnectSSE();
    if (refreshTimer) clearTimeout(refreshTimer);
    if (chart) chart.destroy();
  });
</script>

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Stats</BreadcrumbItem>
    </Breadcrumb>

    <div
      style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem;"
    >
      <div style="display: flex; align-items: center; gap: 0.75rem;">
        <h2 style="margin: 0;">Stats</h2>
        {#if connected}
          <Tag type="green" size="sm">Live</Tag>
        {:else}
          <Tag type="warm-gray" size="sm">Connecting…</Tag>
        {/if}
        {#if eventsReceived > 0}
          <span style="font-size: 0.75rem; color: var(--cds-text-02);">
            {eventsReceived.toLocaleString()} events
          </span>
        {/if}
      </div>

      <Dropdown
        size="sm"
        style="width: 180px;"
        items={scaleOptions}
        selectedId={selectedScale}
        on:select={onScaleChange}
      />
    </div>

    <div id="statschart"></div>
  </Column>
</Row>

{#if chart}
  <Row>
    <Column>
      <div>
        <br />
        <h4>{$_("Top 5 Blocked Requests")}</h4>
        <br />
        {#if topBlockedRows.length > 0}
          <DataTable
            headers={[
              { key: "host", value: "Host" },
              { key: "count", value: "Times requested" },
            ]}
            rows={topBlockedRows}
          />
        {:else}
          <p>
            <i>{$_("Nothing found. Please make some requests.")}</i>
          </p>
        {/if}
      </div>
    </Column>
    <Column>
      <div>
        <br />
        <h4>{$_("Top 5 Requests")}</h4>
        <br />
        {#if topAllRows.length > 0}
          <DataTable
            headers={[
              { key: "host", value: "Host" },
              { key: "count", value: "Times requested" },
            ]}
            rows={topAllRows}
          />
        {:else}
          <p>
            <i>{$_("Nothing found. Please make some requests.")}</i>
          </p>
        {/if}
      </div>
    </Column>
  </Row>
{/if}

{#if !chart}
  <Row>
    <Column>
      <Loading />
    </Column>
  </Row>
{/if}
