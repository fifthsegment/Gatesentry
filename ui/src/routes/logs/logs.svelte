<script lang="ts">
  import { InlineNotification, Search, Tag } from "carbon-components-svelte";
  import { Catalog, Pause, Play } from "carbon-icons-svelte";

  import { format } from "timeago.js";
  import { store } from "../../store/apistore";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";

  let search = "";
  let logs: any[] = [];
  let eventSource: EventSource | null = null;
  let connected = false;
  let paused = false;
  let tick = 0; // bumped every 10s to refresh "time ago" labels
  let tickTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectDelay = 1000; // start at 1s, exponential backoff up to 30s
  const MAX_ENTRIES = 200;

  function getBasePath(): string {
    const bp = (window as any).__GS_BASE_PATH__ || "";
    if (bp === "/") return "";
    return bp;
  }

  /** Format a unix-seconds timestamp to relative time. `_tick` forces Svelte reactivity. */
  function timeAgo(unix: number, _tick: number): string {
    if (!unix) return "";
    return format(unix * 1000);
  }

  /** Build a display-friendly item from a raw log entry */
  const toDisplayItem = (item: any, index: number) => ({
    id: (item.ip || "") + (item.time || "") + index + (item.url || ""),
    ip: item.ip || "",
    timeRaw: item.time || item.Time || 0,
    url: item.url || "",
    urlShort: _.truncate(item.url || "", { length: 60 }),
    responseType:
      item.type === "dns"
        ? item.dnsResponseType || item.DNSResponseType || "dns"
        : item.proxyResponseType || item.ProxyResponseType || "",
  });

  /** Load recent entries via the existing REST endpoint (initial backfill) */
  const loadInitialData = () => {
    $store.api.doCall("/logs/viewlive").then(function (json: any) {
      const items = JSON.parse(json.Items) as Array<any>;
      logs = items.slice(0, MAX_ENTRIES).map(toDisplayItem);
    });
  };

  /** Connect to the SSE stream */
  function connectSSE() {
    if (eventSource) {
      eventSource.close();
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }

    const basePath = getBasePath();
    const jwt = localStorage.getItem("jwt") || "";
    if (!jwt) {
      // No token — don't connect, user needs to log in
      connected = false;
      return;
    }
    const url = `${basePath}/api/logs/stream?token=${encodeURIComponent(jwt)}`;

    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      connected = true;
      reconnectDelay = 1000; // reset backoff on successful connection
    };

    eventSource.onmessage = (event) => {
      if (paused) return;
      try {
        const entry = JSON.parse(event.data);
        const item = toDisplayItem(entry, Date.now());
        logs = [item, ...logs].slice(0, MAX_ENTRIES);
      } catch (e) {
        // ignore malformed messages
      }
    };

    eventSource.addEventListener("reconnect", () => {
      // Server is asking us to reconnect (max duration reached)
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      connected = false;
      reconnectTimer = setTimeout(connectSSE, 500);
    });

    eventSource.onerror = () => {
      connected = false;
      // EventSource enters CLOSED state on HTTP error (e.g. 401).
      // It only auto-reconnects on network errors during an active connection.
      if (eventSource && eventSource.readyState === EventSource.CLOSED) {
        // HTTP error (likely 401 expired JWT) — do NOT auto-reconnect
        // in a tight loop. Use exponential backoff.
        eventSource.close();
        eventSource = null;
        reconnectTimer = setTimeout(connectSSE, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 30000);
      }
      // If readyState is CONNECTING, the browser is auto-reconnecting
      // from a network error — let it handle it.
    };
  }

  function togglePause() {
    paused = !paused;
  }

  $: filteredLogs =
    search.length > 0
      ? logs.filter(
          (item) =>
            item.url.toLowerCase().includes(search.toLowerCase()) ||
            item.ip.includes(search) ||
            item.responseType.toLowerCase().includes(search.toLowerCase()),
        )
      : logs;

  onMount(() => {
    loadInitialData();
    connectSSE();
    // Refresh "time ago" labels every 10 seconds
    tickTimer = setInterval(() => {
      tick++;
    }, 10000);
  });

  onDestroy(() => {
    if (eventSource) {
      eventSource.close();
      eventSource = null;
    }
    if (tickTimer) clearInterval(tickTimer);
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  });

  function tagType(rt: string): string {
    if (rt === "blocked") return "red";
    if (rt === "cached") return "teal";
    if (rt === "forward") return "blue";
    return "gray";
  }
</script>

<div class="gs-page-title">
  <Catalog size={24} />
  <h2>Log viewer</h2>
</div>

<p class="log-desc">Shows the past few requests to GateSentry.</p>

<div class="log-notice">
  <InlineNotification
    kind="info"
    lowContrast
    title="Raspberry Pi / SD Card users:"
    subtitle="To reduce SD card wear, change the log file location to RAM by going to Settings and setting the log location to &quot;/tmp/log.db&quot;. Logs in RAM will not survive a reboot."
    hideCloseButton
  />
</div>

<div class="log-toolbar">
  <div class="log-search">
    <Search
      bind:value={search}
      placeholder="Filter by IP, URL, or type…"
      size="sm"
    />
  </div>
  <button
    class="log-pause"
    on:click={togglePause}
    title={paused ? "Resume" : "Pause"}
  >
    {#if paused}
      <Play size={20} />
    {:else}
      <Pause size={20} />
    {/if}
  </button>
  <span class="log-status" class:log-status--connected={connected}>
    {connected ? "Live" : "Connecting…"}
  </span>
</div>

<div class="gs-card-flush">
  <!-- Header row (desktop only) -->
  <div class="log-header">
    <span class="log-col-ip">IP</span>
    <span class="log-col-time">Time</span>
    <span class="log-col-url">URL</span>
    <span class="log-col-type">Type</span>
  </div>

  <div class="gs-row-list" style="border:none;border-radius:0;">
    {#if filteredLogs.length === 0}
      <div class="gs-row-item">
        <span class="gs-empty">
          {search ? "No matching entries" : "Waiting for log entries…"}
        </span>
      </div>
    {/if}
    {#each filteredLogs as item (item.id)}
      <div class="gs-row-item log-row">
        <!-- Desktop: single horizontal row -->
        <div class="log-row-main">
          <span class="log-col-ip log-ip">{item.ip}</span>
          <span class="log-col-time log-time"
            >{timeAgo(item.timeRaw, tick)}</span
          >
          <span class="log-col-url log-url" title={item.url}
            >{item.urlShort}</span
          >
        </div>
        <!-- Mobile: top row with time + tag -->
        <div class="log-row-top">
          <span class="log-time">{timeAgo(item.timeRaw, tick)}</span>
          <span class="log-tag-slot">
            {#if item.responseType}
              <Tag type={tagType(item.responseType)} size="sm"
                >{item.responseType}</Tag
              >
            {/if}
          </span>
        </div>
        <!-- Mobile: URL -->
        <div class="log-row-url">
          <span class="log-url" title={item.url}>{item.urlShort}</span>
        </div>
        <!-- Mobile: IP -->
        <div class="log-row-ip">
          <span class="log-ip">{item.ip}</span>
        </div>
        <!-- Desktop: tag in its column -->
        <div class="log-col-type">
          {#if item.responseType}
            <Tag type={tagType(item.responseType)} size="sm"
              >{item.responseType}</Tag
            >
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>

<style>
  .log-desc {
    font-size: 0.875rem;
    color: #525252;
    margin: 0 0 0.75rem 0;
  }

  .log-notice {
    margin-bottom: 0.75rem;
  }
  /* Force the Carbon notification to fill its container */
  .log-notice :global(.bx--inline-notification) {
    max-width: 100%;
  }

  .log-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 0.75rem;
  }
  .log-search {
    flex: 1;
    max-width: 360px;
  }
  .log-pause {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: 1px solid #e0e0e0;
    border-radius: 4px;
    background: #fff;
    cursor: pointer;
    color: #525252;
    flex-shrink: 0;
  }
  .log-pause:hover {
    background: #e5e5e5;
  }
  .log-status {
    font-size: 0.75rem;
    color: #a8a8a8;
    white-space: nowrap;
  }
  .log-status--connected {
    color: #198038;
  }

  /* Desktop header row */
  .log-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: #e0e0e0;
    font-size: 0.75rem;
    font-weight: 600;
    color: #525252;
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }

  /* Column widths (desktop) */
  .log-col-ip {
    width: 120px;
    flex-shrink: 0;
  }
  .log-col-time {
    width: 100px;
    flex-shrink: 0;
  }
  .log-col-url {
    flex: 1;
    min-width: 0;
  }
  .log-col-type {
    width: 90px;
    flex-shrink: 0;
    text-align: right;
  }

  /* Each log row */
  .log-row-main {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;
  }
  .log-ip {
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.8125rem;
    color: #161616;
  }
  .log-time {
    font-size: 0.8125rem;
    color: #6f6f6f;
  }
  .log-url {
    font-size: 0.8125rem;
    color: #393939;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Mobile-only card rows — hidden on desktop */
  .log-row-top,
  .log-row-url,
  .log-row-ip {
    display: none;
  }

  .log-row-top {
    align-items: center;
    justify-content: space-between;
    width: 100%;
  }
  .log-tag-slot {
    flex-shrink: 0;
  }

  .log-row-url {
    width: 100%;
  }
  .log-row-url .log-url {
    word-break: break-all;
    white-space: normal;
  }
  .log-row-ip {
    width: 100%;
  }

  /* ── Mobile ── */
  @media (max-width: 671px) {
    .log-search {
      max-width: none;
    }
    .log-header {
      display: none;
    }
    /* Hide desktop row parts */
    .log-row-main,
    .log-col-type {
      display: none;
    }
    /* Show mobile card parts */
    .log-row-top,
    .log-row-url,
    .log-row-ip {
      display: flex;
    }
    .log-row {
      flex-direction: column;
      align-items: flex-start;
      gap: 4px;
    }
  }
</style>
