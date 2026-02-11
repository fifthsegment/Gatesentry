<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Dropdown,
    Loading,
    Row,
    Tab,
    TabContent,
    Tabs,
    Tag,
    Tile,
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

  interface RawEvent {
    ts: number;
    domain: string;
    blocked: boolean;
  }

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
   * Raw SSE request events, stored so we can re-bucket dynamically
   * when the user changes the time-scale dropdown.  Pruned periodically.
   */
  let rawEvents: RawEvent[] = [];

  let eventSource: EventSource | null = null;
  let connected = false;
  let eventsReceived = 0;

  // ---------- Helpers ----------

  /** Map UI scale id → API query parameters */
  function scaleToApiParams(scale: string): { seconds: number; group: string } {
    if (scale === "1h") return { seconds: 3600, group: "minute" };
    if (scale === "24h") return { seconds: 86400, group: "hour" };
    return { seconds: 604800, group: "day" }; // 7d
  }

  /** Fetch historical data from BuntDB for the given scale. */
  async function fetchHistory(scale: string): Promise<ResponseData | null> {
    try {
      const { seconds, group } = scaleToApiParams(scale);
      const json = (await $store.api.doCall(
        `/stats/byUrl?seconds=${seconds}&group=${group}`,
      )) as ResponseData;
      return json || null;
    } catch (err) {
      console.error("Error fetching historical stats:", err);
      return null;
    }
  }

  /**
   * Return a LOCAL-time bucket key for a given timestamp + scale.
   * Using local date components avoids UTC ↔ local mismatches that
   * cause events to land in the wrong bucket for users not in UTC.
   * Keys are ISO-like strings that sort chronologically.
   */
  function bucketKey(ts: number, scale: string): string {
    const d = new Date(ts);
    const Y = d.getFullYear();
    const M = String(d.getMonth() + 1).padStart(2, "0");
    const D = String(d.getDate()).padStart(2, "0");
    const h = String(d.getHours()).padStart(2, "0");
    const m = String(d.getMinutes()).padStart(2, "0");

    if (scale === "1h") return `${Y}-${M}-${D}T${h}:${m}`; // per-minute
    if (scale === "24h") return `${Y}-${M}-${D}T${h}`; // per-hour
    return `${Y}-${M}-${D}`; // per-day
  }

  /**
   * Parse a bucket key back into a Date for the chart axis.
   * All keys are LOCAL-time strings (no "Z" suffix), so the Date
   * constructor interprets them in the browser's local timezone.
   */
  function bucketToDate(key: string, scale: string): Date {
    if (scale === "1h") return new Date(key + ":00"); // "YYYY-MM-DDTHH:MM" → :00
    if (scale === "24h") return new Date(key + ":00:00"); // "YYYY-MM-DDTHH"   → :00:00
    return new Date(key + "T12:00:00"); // noon local (avoids DST edge)
  }

  /**
   * Merge historical + real-time data and produce chart data + top-5 tables.
   * Real-time events are filtered to the selected time window and bucketed
   * on the fly, so changing the scale instantly re-groups the data.
   */
  function buildView(
    hist: ResponseData | null,
    events: RawEvent[],
    scale: string,
  ) {
    const seriesAll = new Map<string, number>();
    const seriesBlocked = new Map<string, number>();
    const allCounts = new Map<string, number>();
    const blockedCounts = new Map<string, number>();

    // 1. Historical data from BuntDB (used for ALL scales — the API
    //    returns bucket keys that match our local-time bucket format).
    if (hist) {
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

    // 2. Real-time SSE events — filter to the selected time window, then bucket
    const now = Date.now();
    const cutoff =
      scale === "7d"
        ? now - 7 * 86_400_000
        : scale === "24h"
        ? now - 86_400_000
        : now - 3_600_000;

    for (const evt of events) {
      if (evt.ts < cutoff) continue;

      const key = bucketKey(evt.ts, scale);

      seriesAll.set(key, (seriesAll.get(key) || 0) + 1);
      allCounts.set(evt.domain, (allCounts.get(evt.domain) || 0) + 1);

      if (evt.blocked) {
        seriesBlocked.set(key, (seriesBlocked.get(key) || 0) + 1);
        blockedCounts.set(evt.domain, (blockedCounts.get(evt.domain) || 0) + 1);
      }
    }

    // 3. Build chart array, sorted by bucket key (ISO-like keys sort correctly)
    const allBuckets = new Set([...seriesAll.keys(), ...seriesBlocked.keys()]);
    const sorted = [...allBuckets].sort();
    const chartData: { group: string; date: Date; value: number }[] = [];

    for (const b of sorted) {
      const d = bucketToDate(b, scale);
      if (seriesAll.has(b))
        chartData.push({
          group: "All Requests",
          date: d,
          value: seriesAll.get(b)!,
        });
      if (seriesBlocked.has(b))
        chartData.push({
          group: "Blocked Requests",
          date: d,
          value: seriesBlocked.get(b)!,
        });
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

  // ========== DNS CACHE TAB STATE ==========

  interface CacheSnapshot {
    hits: number;
    misses: number;
    inserts: number;
    evictions: number;
    expired: number;
    entries: number;
    max_entries: number;
    size_bytes: number;
    hit_rate_pct: number;
  }

  interface CacheEvent {
    ts: number;
    type: "hit" | "miss" | "evict" | "expire";
  }

  /** Gauge-like values: entries, max_entries, size_bytes (current state). */
  let cacheSnap: CacheSnapshot = {
    hits: 0,
    misses: 0,
    inserts: 0,
    evictions: 0,
    expired: 0,
    entries: 0,
    max_entries: 10000,
    size_bytes: 0,
    hit_rate_pct: 0,
  };

  /** Live cache events since page load (current minute, not yet in a snapshot). */
  let cacheEvents: CacheEvent[] = [];

  /**
   * Per-minute snapshots from BuntDB.  The backend resets counters after
   * each snapshot, so each entry IS the per-minute count (not cumulative).
   */
  interface CacheMinuteDelta {
    time: string;
    timeMs: number;
    hits: number;
    misses: number;
    inserts: number;
    evictions: number;
    expired: number;
  }
  let cacheHistory: CacheMinuteDelta[] = [];

  /** Rolling 1-hour totals derived from history snapshots + live events. */
  let hourlyTotals = {
    hits: 0,
    misses: 0,
    inserts: 0,
    evictions: 0,
    expired: 0,
    hitRate: 0,
  };

  let cacheChart: any = null;
  let cacheChartHolder: HTMLElement;

  /** Active tab index: 0 = Traffic, 1 = DNS Cache */
  let activeTab = 0;

  // ---------- Cache helpers ----------

  /** Fetch the current cache stats snapshot from the REST API. */
  async function fetchCacheStats(): Promise<CacheSnapshot | null> {
    try {
      const json = (await $store.api.doCall(
        "/dns/cache/stats",
      )) as CacheSnapshot;
      return json || null;
    } catch (err) {
      console.error("Error fetching cache stats:", err);
      return null;
    }
  }

  /** Fetch per-minute snapshots from BuntDB and compute deltas. */
  async function fetchCacheHistory(): Promise<CacheMinuteDelta[]> {
    try {
      console.log("[fetchCacheHistory] Fetching history...");
      const snapshots = (await $store.api.doCall(
        "/dns/cache/stats/history?minutes=60",
      )) as { time: string; time_unix_ms: number; stats: CacheSnapshot }[];

      console.log(
        "[fetchCacheHistory] Received snapshots:",
        snapshots?.length || 0,
        snapshots,
      );

      if (!snapshots || snapshots.length === 0) {
        console.warn("[fetchCacheHistory] No snapshots returned");
        return [];
      }

      // Each snapshot stores per-minute counts (the backend resets
      // counters after each snapshot), so no delta computation needed.
      const result = snapshots.map((s) => ({
        time: s.time,
        timeMs: s.time_unix_ms,
        hits: s.stats.hits || 0,
        misses: s.stats.misses || 0,
        inserts: s.stats.inserts || 0,
        evictions: s.stats.evictions || 0,
        expired: s.stats.expired || 0,
      }));
      console.log(
        "[fetchCacheHistory] Mapped to deltas:",
        result.length,
        result.slice(0, 3),
      );
      return result;
    } catch (err) {
      console.error("[fetchCacheHistory] Error:", err);
      return [];
    }
  }

  /**
   * Build cache chart data as a sliding 60-minute window.
   *
   * Data sources (merged by minute key):
   * 1. Historical deltas from BuntDB snapshots (survives page reload)
   * 2. Live SSE query events since page load (fills current minute)
   *
   * Every minute slot from (now − 60 min) to now is present — even if
   * the count is 0 — so the x-axis always shows a full hour and scrolls
   * left as new events arrive.
   */
  function buildCacheChartData(): {
    group: string;
    date: Date;
    value: number;
  }[] {
    const now = Date.now();
    const cutoff = now - 3_600_000;

    // 1. Seed from historical deltas (BuntDB snapshots)
    const hitsMap = new Map<string, number>();
    const missesMap = new Map<string, number>();

    for (const d of cacheHistory) {
      if (d.timeMs < cutoff) continue;
      hitsMap.set(d.time, (hitsMap.get(d.time) || 0) + d.hits);
      missesMap.set(d.time, (missesMap.get(d.time) || 0) + d.misses);
    }

    // 2. Overlay live SSE events (fills current minute and any gap since last snapshot)
    for (const evt of cacheEvents) {
      if (evt.ts < cutoff) continue;
      if (evt.type !== "hit" && evt.type !== "miss") continue;
      const d = new Date(evt.ts);
      const key = minuteKey(d);
      if (evt.type === "hit") {
        hitsMap.set(key, (hitsMap.get(key) || 0) + 1);
      } else {
        missesMap.set(key, (missesMap.get(key) || 0) + 1);
      }
    }

    // 3. Generate all 60 minute slots so the axis is always full
    const data: { group: string; date: Date; value: number }[] = [];
    const start = new Date(cutoff);
    start.setSeconds(0, 0); // round down to minute

    for (let t = start.getTime(); t <= now; t += 60_000) {
      const d = new Date(t);
      const key = minuteKey(d);
      data.push({
        group: "Cache Misses",
        date: d,
        value: missesMap.get(key) || 0,
      });
      data.push({ group: "Cache Hits", date: d, value: hitsMap.get(key) || 0 });
    }

    return data;
  }

  /** Format a Date into a local-time minute key "YYYY-MM-DDTHH:MM" */
  function minuteKey(d: Date): string {
    const Y = d.getFullYear();
    const M = String(d.getMonth() + 1).padStart(2, "0");
    const D = String(d.getDate()).padStart(2, "0");
    const h = String(d.getHours()).padStart(2, "0");
    const m = String(d.getMinutes()).padStart(2, "0");
    return `${Y}-${M}-${D}T${h}:${m}`;
  }

  function makeCacheChartOptions() {
    const locale = navigator.language || "en-US";
    const now = new Date();
    const oneHourAgo = new Date(now.getTime() - 3_600_000);

    return {
      title: "Cache hits vs misses (past hour)",
      axes: {
        bottom: {
          title: "Time",
          mapsTo: "date",
          scaleType: "time",
          domain: [oneHourAgo, now],
          ticks: {
            formatter: (d: Date) => {
              if (!(d instanceof Date) || isNaN(d.getTime())) return "";
              return d.toLocaleTimeString(locale, {
                hour: "2-digit",
                minute: "2-digit",
              });
            },
          },
        },
        left: {
          title: "Queries",
          mapsTo: "value",
          scaleType: "linear",
        },
      },
      height: "400px",
      toolbar: { enabled: false },
      color: {
        scale: {
          "Cache Hits": "#198038",
          "Cache Misses": "#da1e28",
        },
      },
      legend: { alignment: "center" },
      curve: "curveMonotoneX",
    };
  }

  /** Compute rolling 1-hour totals from history snapshots + live events. */
  function computeHourlyTotals() {
    const cutoff = Date.now() - 3_600_000;
    let hits = 0,
      misses = 0,
      inserts = 0,
      evictions = 0,
      expired = 0;

    for (const d of cacheHistory) {
      if (d.timeMs < cutoff) continue;
      hits += d.hits;
      misses += d.misses;
      inserts += d.inserts;
      evictions += d.evictions;
      expired += d.expired;
    }

    for (const evt of cacheEvents) {
      if (evt.ts < cutoff) continue;
      switch (evt.type) {
        case "hit":
          hits++;
          break;
        case "miss":
          misses++;
          break;
        case "evict":
          evictions++;
          break;
        case "expire":
          expired++;
          break;
      }
    }

    const total = hits + misses;
    hourlyTotals = {
      hits,
      misses,
      inserts,
      evictions,
      expired,
      hitRate: total > 0 ? (hits / total) * 100 : 0,
    };
  }

  function refreshCacheChart() {
    if (!cacheChart) return;
    const data = buildCacheChartData();
    // Slide the x-axis domain so "now" is always the right edge
    cacheChart.model.setOptions(makeCacheChartOptions());
    cacheChart.model.setData(data);
    computeHourlyTotals();
  }

  /** Throttle cache chart updates */
  let cacheRefreshTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleCacheRefresh() {
    if (cacheRefreshTimer) return;
    cacheRefreshTimer = setTimeout(() => {
      cacheRefreshTimer = null;
      refreshCacheChart();
    }, 500);
  }

  /** Format bytes into a human-readable string. */
  function formatBytes(bytes: number): string {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  }

  // ---------- Reactive rendering ----------

  let chartData: { group: string; date: Date; value: number }[] = [];
  let topAllRows: { id: string; host: string; count: number }[] = [];
  let topBlockedRows: { id: string; host: string; count: number }[] = [];

  function refresh() {
    const result = buildView(historicalData, rawEvents, selectedScale);
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

        // ---------- Traffic tab: request events ----------
        if (evt.type === "request") {
          eventsReceived++;
          rawEvents.push({
            ts: evt.ts,
            domain: evt.domain || "unknown",
            blocked: !!evt.blocked,
          });
          scheduleRefresh();
        }

        // ---------- Cache tab: query events (hits / misses) ----------
        if (evt.type === "query") {
          cacheEvents.push({ ts: evt.ts, type: evt.hit ? "hit" : "miss" });
          cacheSnap = {
            ...cacheSnap,
            entries: evt.cache_size ?? cacheSnap.entries,
          };
          scheduleCacheRefresh();
        }

        // ---------- Cache tab: eviction events ----------
        if (evt.type === "evict") {
          cacheEvents.push({ ts: evt.ts, type: "evict" });
          cacheSnap = {
            ...cacheSnap,
            entries: evt.cache_size ?? cacheSnap.entries,
          };
          scheduleCacheRefresh();
        }

        // ---------- Cache tab: expiry events ----------
        if (evt.type === "expire") {
          cacheEvents.push({ ts: evt.ts, type: "expire" });
          cacheSnap = {
            ...cacheSnap,
            entries: evt.cache_size ?? cacheSnap.entries,
          };
          scheduleCacheRefresh();
        }
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
    const now = new Date();

    // Explicit x-axis domain so the chart always spans the full window,
    // even when there's little or no data in parts of the range.
    let domain: [Date, Date];
    if (scale === "1h") {
      domain = [new Date(now.getTime() - 3_600_000), now];
    } else if (scale === "24h") {
      domain = [new Date(now.getTime() - 86_400_000), now];
    } else {
      domain = [new Date(now.getTime() - 7 * 86_400_000), now];
    }

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
          domain,
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

  // When the scale dropdown changes, fetch matching history & update chart
  async function onScaleChange(e: CustomEvent) {
    selectedScale = e.detail.selectedId;

    if (chart) {
      chart.model.setOptions(makeChartOptions(selectedScale));
    }

    // Fetch historical data at the right granularity for this scale
    historicalData = await fetchHistory(selectedScale);
    refresh();
  }

  // ---------- Lifecycle ----------

  /** Prune raw events older than 7 days to bound memory. */
  let pruneTimer: ReturnType<typeof setInterval> | null = null;

  onMount(async () => {
    // ---------- Traffic chart ----------
    chartHolder = document.getElementById("statschart") as HTMLElement;
    if (!chartHolder) throw new Error("Could not find chart holder element");

    // @ts-ignore
    chart = new AreaChart(chartHolder, {
      data: [],
      // @ts-ignore
      options: makeChartOptions(selectedScale),
    });

    // 1. Fetch historical data for the default scale (7 days)
    historicalData = await fetchHistory(selectedScale);
    refresh();

    // 2. Fetch initial cache stats snapshot + historical deltas
    const [snap, history] = await Promise.all([
      fetchCacheStats(),
      fetchCacheHistory(),
    ]);
    if (history) cacheHistory = history;
    // Use the live snapshot only for gauge values (entries, max_entries, size_bytes).
    if (snap) cacheSnap = snap;
    // Compute the rolling 1-hour totals for the metric tiles.
    computeHourlyTotals();

    // Now init the cache chart (the #cachechart div is always in the DOM
    // because Carbon TabContent renders all tabs, it just hides inactive ones).
    cacheChartHolder = document.getElementById("cachechart") as HTMLElement;
    if (cacheChartHolder) {
      // @ts-ignore
      cacheChart = new AreaChart(cacheChartHolder, {
        data: buildCacheChartData(),
        // @ts-ignore
        options: makeCacheChartOptions(),
      });
    }

    // 3. Open SSE stream for real-time events (shared by both tabs)
    connectSSE();

    // 4. Prune stale events every 60 seconds to bound memory
    pruneTimer = setInterval(() => {
      const cutoff7d = Date.now() - 7 * 86_400_000;
      rawEvents = rawEvents.filter((e) => e.ts >= cutoff7d);

      const cutoff1h = Date.now() - 3_600_000;
      cacheEvents = cacheEvents.filter((e) => e.ts >= cutoff1h);
    }, 60_000);
  });

  onDestroy(() => {
    disconnectSSE();
    if (refreshTimer) clearTimeout(refreshTimer);
    if (cacheRefreshTimer) clearTimeout(cacheRefreshTimer);
    if (pruneTimer) clearInterval(pruneTimer);
    if (chart) chart.destroy();
    if (cacheChart) cacheChart.destroy();
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
    </div>

    <Tabs bind:selected={activeTab}>
      <Tab label="Traffic" />
      <Tab label="DNS Cache" />
      <svelte:fragment slot="content">
        <!-- ==================== TAB 0: TRAFFIC ==================== -->
        <TabContent>
          <div
            style="display: flex; justify-content: flex-end; margin-bottom: 1rem;"
          >
            <Dropdown
              size="sm"
              style="width: 180px;"
              items={scaleOptions}
              selectedId={selectedScale}
              on:select={onScaleChange}
            />
          </div>

          <div id="statschart"></div>

          {#if chart}
            <Row style="margin-top: 1.5rem;">
              <Column>
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
              </Column>
              <Column>
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
              </Column>
            </Row>
          {/if}

          {#if !chart}
            <Loading />
          {/if}
        </TabContent>

        <!-- ==================== TAB 1: DNS CACHE ==================== -->
        <TabContent>
          <!-- Stat tiles row -->
          <div class="cache-tiles">
            <Tile class="cache-tile">
              <div class="tile-label">Hit Rate (1h)</div>
              <div class="tile-value">{hourlyTotals.hitRate.toFixed(1)}%</div>
              <div class="tile-sub">
                {(hourlyTotals.hits + hourlyTotals.misses).toLocaleString()} queries
                in the last hour
              </div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Cache Entries</div>
              <div class="tile-value">
                {cacheSnap.entries.toLocaleString()}
                <span class="tile-max"
                  >/ {cacheSnap.max_entries.toLocaleString()}</span
                >
              </div>
              <div class="tile-sub">{formatBytes(cacheSnap.size_bytes)}</div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Hits (1h)</div>
              <div class="tile-value tile-green">
                {hourlyTotals.hits.toLocaleString()}
              </div>
              <div class="tile-sub">served from cache</div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Misses (1h)</div>
              <div class="tile-value tile-red">
                {hourlyTotals.misses.toLocaleString()}
              </div>
              <div class="tile-sub">forwarded to upstream</div>
            </Tile>
          </div>

          <!-- Eviction / expiry summary row -->
          <div class="cache-tiles" style="margin-bottom: 1rem;">
            <Tile class="cache-tile">
              <div class="tile-label">Evictions (1h)</div>
              <div class="tile-value tile-amber">
                {hourlyTotals.evictions.toLocaleString()}
              </div>
              <div class="tile-sub">
                {#if hourlyTotals.evictions > 0 && cacheSnap.entries >= cacheSnap.max_entries * 0.9}
                  <Tag type="red" size="sm"
                    >Cache full — consider increasing max entries</Tag
                  >
                {:else if hourlyTotals.evictions > 0}
                  capacity pressure detected
                {:else}
                  no eviction pressure
                {/if}
              </div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Expired (1h)</div>
              <div class="tile-value">
                {hourlyTotals.expired.toLocaleString()}
              </div>
              <div class="tile-sub">entries removed by TTL expiry</div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Inserts (1h)</div>
              <div class="tile-value">
                {hourlyTotals.inserts.toLocaleString()}
              </div>
              <div class="tile-sub">entries added to cache</div>
            </Tile>

            <Tile class="cache-tile">
              <div class="tile-label">Efficiency (1h)</div>
              <div class="tile-value">
                {#if hourlyTotals.hits + hourlyTotals.misses > 0}
                  {(
                    (hourlyTotals.hits /
                      (hourlyTotals.hits + hourlyTotals.misses)) *
                    100
                  ).toFixed(1)}%
                {:else}
                  —
                {/if}
              </div>
              <div class="tile-sub">
                {#if hourlyTotals.hitRate >= 70}
                  <Tag type="green" size="sm">Healthy</Tag>
                {:else if hourlyTotals.hitRate >= 40}
                  <Tag type="blue" size="sm">Warming up</Tag>
                {:else if hourlyTotals.hits + hourlyTotals.misses > 100}
                  <Tag type="red" size="sm">Low — check cache config</Tag>
                {:else}
                  <Tag type="warm-gray" size="sm">Insufficient data</Tag>
                {/if}
              </div>
            </Tile>
          </div>

          <!-- Stacked area chart: hits vs misses over time -->
          <div id="cachechart"></div>
        </TabContent>
      </svelte:fragment>
    </Tabs>
  </Column>
</Row>

<style>
  .cache-tiles {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 1rem;
    margin-bottom: 1.5rem;
    margin-top: 1rem;
  }

  @media (max-width: 672px) {
    .cache-tiles {
      grid-template-columns: repeat(2, 1fr);
    }
  }

  .tile-label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.32px;
    color: var(--cds-text-02, #525252);
    margin-bottom: 0.25rem;
  }

  .tile-value {
    font-size: 2rem;
    font-weight: 300;
    line-height: 1.2;
    color: var(--cds-text-01, #161616);
  }

  .tile-max {
    font-size: 1rem;
    color: var(--cds-text-02, #525252);
  }

  .tile-sub {
    font-size: 0.75rem;
    color: var(--cds-text-02, #525252);
    margin-top: 0.25rem;
  }

  .tile-green {
    color: #198038;
  }

  .tile-red {
    color: #da1e28;
  }

  .tile-amber {
    color: #f1c21b;
  }

  /* Lower the area fill opacity on the cache chart so overlapping
     red (misses) and green (hits) areas are both clearly visible. */
  :global(#cachechart .area) {
    fill-opacity: 0.15 !important;
  }
</style>
