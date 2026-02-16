<script lang="ts">
  import { InlineNotification, Search, Tag } from "carbon-components-svelte";
  import { Catalog, Pause, Play } from "carbon-icons-svelte";

  import { format } from "timeago.js";
  import { store } from "../../store/apistore";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";

  // ── Types ──
  interface LogItem {
    id: string;
    ip: string;
    timeRaw: number;
    url: string;
    urlShort: string;
    entryType: string;
    action: string;
    actionLabel: string;
    ruleName: string;
  }

  // ── State ──
  let search = "";
  let logs: LogItem[] = [];
  let eventSource: EventSource | null = null;
  let connected = false;
  let paused = false;
  let tick = 0;
  let tickTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectDelay = 1000;
  let loading = false;
  let totalAll = 0;

  // Filter state
  let typeFilter: "all" | "dns" | "proxy" = "all";
  let actionFilter: "all" | "blocked" | "allowed" = "all";
  let timeRange = 300;

  const MAX_ENTRIES = 500;

  const TIME_OPTIONS = [
    { label: "5 min", value: 300 },
    { label: "1 hour", value: 3600 },
    { label: "24 hours", value: 86400 },
    { label: "7 days", value: 604800 },
  ];

  function getBasePath(): string {
    const bp = (window as any).__GS_BASE_PATH__ || "";
    if (bp === "/") return "";
    return bp;
  }

  function timeAgo(unix: number, _tick: number): string {
    if (!unix) return "";
    return format(unix * 1000);
  }

  function proxyActionLabel(action: string): string {
    const labels: Record<string, string> = {
      blocked_url: "Blocked (Domain/URL)",
      blocked_text_content: "Blocked (Keywords)",
      blocked_media_content: "Blocked (Media)",
      blocked_file_type: "Blocked (File Type)",
      blocked_time: "Blocked (Time)",
      blocked_internet_for_user: "Blocked (User)",
      auth_failure: "Auth Failure",
      "ssl-bump": "MITM",
      ssldirect: "Passthrough",
      filternone: "Allowed",
      filtererror: "Error",
    };
    return labels[action] || action;
  }

  function dnsActionLabel(action: string): string {
    const labels: Record<string, string> = {
      blocked: "Blocked",
      cached: "Cached",
      forward: "Forwarded",
    };
    return labels[action] || action;
  }

  function isBlockedAction(entryType: string, action: string): boolean {
    if (entryType === "dns") return action === "blocked";
    return action.startsWith("blocked_") || action === "auth_failure";
  }

  function tagType(entryType: string, action: string): string {
    if (action === "auth_failure") return "magenta";
    if (isBlockedAction(entryType, action)) return "red";
    if (entryType === "dns") {
      if (action === "cached") return "teal";
      if (action === "forward") return "blue";
      return "gray";
    }
    if (action === "ssl-bump") return "purple";
    if (action === "ssldirect") return "blue";
    if (action === "filternone") return "green";
    return "gray";
  }

  function typeTagColor(t: string): string {
    return t === "dns" ? "blue" : "purple";
  }

  function toDisplayItem(item: any, index: number): LogItem {
    const entryType = item.type || "";
    const action =
      entryType === "dns"
        ? item.dnsResponseType || item.DNSResponseType || "dns"
        : item.proxyResponseType || item.ProxyResponseType || "";
    const label =
      entryType === "dns" ? dnsActionLabel(action) : proxyActionLabel(action);
    const url = item.url || "";
    return {
      id: (item.ip || "") + (item.time || "") + index + url,
      ip: item.ip || "",
      timeRaw: item.time || item.Time || 0,
      url,
      urlShort: _.truncate(url, { length: 120 }),
      entryType,
      action,
      actionLabel: label,
      ruleName: item.ruleName || item.rule_name || "",
    };
  }

  function fromQueryEntry(e: any): LogItem {
    const url = e.url || "";
    return {
      id: (e.ip || "") + (e.time || "") + url + Math.random(),
      ip: e.ip || "",
      timeRaw: e.time || 0,
      url,
      urlShort: _.truncate(url, { length: 120 }),
      entryType: e.type || "",
      action: e.action || "",
      actionLabel: e.action_label || e.action || "",
      ruleName: e.rule_name || "",
    };
  }

  async function fetchFilteredLogs() {
    loading = true;
    try {
      const params = new URLSearchParams();
      params.set("seconds", String(timeRange));
      params.set("limit", String(MAX_ENTRIES));
      if (typeFilter !== "all") params.set("type", typeFilter);
      if (actionFilter !== "all") params.set("filter", actionFilter);
      if (search) params.set("search", search);

      const data = await $store.api.doCall(`/logs/query?${params.toString()}`);
      if (data && data.entries) {
        logs = data.entries.map(fromQueryEntry);
        totalAll = data.total_all || 0;
      }
    } catch (e) {
      console.error("Failed to fetch logs:", e);
    } finally {
      loading = false;
    }
  }

  const debouncedFetch = _.debounce(fetchFilteredLogs, 400);

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
      connected = false;
      return;
    }
    const url = `${basePath}/api/logs/stream?token=${encodeURIComponent(jwt)}`;
    eventSource = new EventSource(url);

    eventSource.onopen = () => {
      connected = true;
      reconnectDelay = 1000;
    };

    eventSource.onmessage = (event) => {
      if (paused) return;
      try {
        const entry = JSON.parse(event.data);
        const item = toDisplayItem(entry, Date.now());

        if (typeFilter !== "all" && item.entryType !== typeFilter) return;
        if (
          actionFilter === "blocked" &&
          !isBlockedAction(item.entryType, item.action)
        )
          return;
        if (
          actionFilter === "allowed" &&
          isBlockedAction(item.entryType, item.action)
        )
          return;
        if (search) {
          const s = search.toLowerCase();
          if (
            !item.url.toLowerCase().includes(s) &&
            !item.ip.toLowerCase().includes(s) &&
            !item.action.toLowerCase().includes(s) &&
            !item.ruleName.toLowerCase().includes(s)
          )
            return;
        }

        logs = [item, ...logs].slice(0, MAX_ENTRIES);
      } catch (e) {
        // ignore
      }
    };

    eventSource.addEventListener("reconnect", () => {
      if (eventSource) {
        eventSource.close();
        eventSource = null;
      }
      connected = false;
      reconnectTimer = setTimeout(connectSSE, 500);
    });

    eventSource.onerror = () => {
      connected = false;
      if (eventSource && eventSource.readyState === EventSource.CLOSED) {
        eventSource.close();
        eventSource = null;
        reconnectTimer = setTimeout(connectSSE, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 30000);
      }
    };
  }

  function togglePause() {
    paused = !paused;
  }

  function setTypeFilter(t: "all" | "dns" | "proxy") {
    typeFilter = t;
    fetchFilteredLogs();
  }

  function setActionFilter(a: "all" | "blocked" | "allowed") {
    actionFilter = a;
    fetchFilteredLogs();
  }

  function setTimeRange(s: number) {
    timeRange = s;
    fetchFilteredLogs();
  }

  $: if (search !== undefined) {
    debouncedFetch();
  }

  onMount(() => {
    fetchFilteredLogs();
    connectSSE();
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
</script>

<div class="gs-page-title">
  <Catalog size={24} />
  <h2>Log viewer</h2>
</div>

<p class="log-desc">
  Real-time and historical log entries. Use filters to find blocked requests.
</p>

<div class="log-notice">
  <InlineNotification
    kind="info"
    lowContrast
    title="Raspberry Pi / SD Card users:"
    subtitle={'To reduce SD card wear, change the log file location to RAM by going to Settings and setting the log location to "/tmp/log.db". Logs in RAM will not survive a reboot.'}
    hideCloseButton
  />
</div>

<!-- Filter bar -->
<div class="log-filters">
  <div class="filter-group">
    <span class="filter-label">Type</span>
    <div class="filter-pills">
      <button
        class="filter-pill"
        class:filter-pill--active={typeFilter === "all"}
        on:click={() => setTypeFilter("all")}>All</button
      >
      <button
        class="filter-pill"
        class:filter-pill--active={typeFilter === "dns"}
        on:click={() => setTypeFilter("dns")}>DNS</button
      >
      <button
        class="filter-pill"
        class:filter-pill--active={typeFilter === "proxy"}
        on:click={() => setTypeFilter("proxy")}>Proxy</button
      >
    </div>
  </div>

  <div class="filter-group">
    <span class="filter-label">Action</span>
    <div class="filter-pills">
      <button
        class="filter-pill"
        class:filter-pill--active={actionFilter === "all"}
        on:click={() => setActionFilter("all")}>All</button
      >
      <button
        class="filter-pill filter-pill--blocked"
        class:filter-pill--active={actionFilter === "blocked"}
        on:click={() => setActionFilter("blocked")}>Blocked</button
      >
      <button
        class="filter-pill"
        class:filter-pill--active={actionFilter === "allowed"}
        on:click={() => setActionFilter("allowed")}>Allowed</button
      >
    </div>
  </div>

  <div class="filter-group">
    <span class="filter-label">Window</span>
    <div class="filter-pills">
      {#each TIME_OPTIONS as opt}
        <button
          class="filter-pill"
          class:filter-pill--active={timeRange === opt.value}
          on:click={() => setTimeRange(opt.value)}>{opt.label}</button
        >
      {/each}
    </div>
  </div>
</div>

<!-- Search + controls -->
<div class="log-toolbar">
  <div class="log-search">
    <Search
      bind:value={search}
      placeholder="Filter by IP, URL, action, or rule name…"
      size="sm"
    />
  </div>
  <button
    class="log-pause"
    on:click={togglePause}
    title={paused ? "Resume live updates" : "Pause live updates"}
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
  <span class="log-count">
    {#if loading}
      Loading…
    {:else}
      {logs.length}{totalAll > logs.length ? ` of ${totalAll}` : ""} entries
    {/if}
  </span>
</div>

<div class="gs-card-flush">
  <div class="log-header">
    <span class="log-col-type-badge">Type</span>
    <span class="log-col-ip">User / IP</span>
    <span class="log-col-time">Time</span>
    <span class="log-col-url">URL / Domain</span>
    <span class="log-col-action">Action</span>
  </div>

  <div class="gs-row-list" style="border:none;border-radius:0;">
    {#if logs.length === 0 && !loading}
      <div class="gs-row-item">
        <span class="gs-empty">
          {search || typeFilter !== "all" || actionFilter !== "all"
            ? "No matching entries"
            : "Waiting for log entries…"}
        </span>
      </div>
    {/if}
    {#each logs as item (item.id)}
      <div
        class="gs-row-item log-row"
        class:log-row--blocked={isBlockedAction(item.entryType, item.action)}
      >
        <!-- Desktop layout -->
        <div class="log-row-main">
          <span class="log-col-type-badge">
            <Tag type={typeTagColor(item.entryType)} size="sm"
              >{item.entryType.toUpperCase()}</Tag
            >
          </span>
          <span class="log-col-ip log-ip">{item.ip}</span>
          <span class="log-col-time log-time"
            >{timeAgo(item.timeRaw, tick)}</span
          >
          <span class="log-col-url log-url" title={item.url}>
            {item.urlShort}
            {#if item.ruleName}
              <span class="log-rule" title="Matched rule: {item.ruleName}"
                >⤷ {item.ruleName}</span
              >
            {/if}
          </span>
        </div>
        <!-- Mobile layout -->
        <div class="log-row-top">
          <span class="log-time">{timeAgo(item.timeRaw, tick)}</span>
          <span class="log-tag-slot">
            <Tag type={typeTagColor(item.entryType)} size="sm"
              >{item.entryType.toUpperCase()}</Tag
            >
            {#if item.action}
              <Tag type={tagType(item.entryType, item.action)} size="sm"
                >{item.actionLabel}</Tag
              >
            {/if}
          </span>
        </div>
        <div class="log-row-url">
          <span class="log-url" title={item.url}>{item.urlShort}</span>
          {#if item.ruleName}
            <span class="log-rule">⤷ Rule: {item.ruleName}</span>
          {/if}
        </div>
        <div class="log-row-ip">
          <span class="log-ip">{item.ip}</span>
        </div>
        <!-- Desktop action column -->
        <div class="log-col-action">
          {#if item.action}
            <Tag type={tagType(item.entryType, item.action)} size="sm"
              >{item.actionLabel}</Tag
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
  .log-notice :global(.bx--inline-notification) {
    max-width: 100%;
  }

  /* ── Filter bar ── */
  .log-filters {
    display: flex;
    flex-wrap: wrap;
    gap: 16px;
    margin-bottom: 0.75rem;
    padding: 12px 16px;
    background: #f4f4f4;
    border-radius: 4px;
  }
  .filter-group {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .filter-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: #525252;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }
  .filter-pills {
    display: flex;
    gap: 4px;
  }
  .filter-pill {
    padding: 4px 12px;
    font-size: 0.8125rem;
    border: 1px solid #c6c6c6;
    border-radius: 16px;
    background: #fff;
    color: #525252;
    cursor: pointer;
    transition: all 0.15s;
  }
  .filter-pill:hover {
    background: #e5e5e5;
  }
  .filter-pill--active {
    background: #0f62fe;
    color: #fff;
    border-color: #0f62fe;
  }
  .filter-pill--active:hover {
    background: #0353e9;
  }
  .filter-pill--blocked.filter-pill--active {
    background: #da1e28;
    border-color: #da1e28;
  }
  .filter-pill--blocked.filter-pill--active:hover {
    background: #ba1b23;
  }

  /* ── Toolbar ── */
  .log-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 0.75rem;
  }
  .log-search {
    flex: 1;
    max-width: 400px;
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
  .log-count {
    font-size: 0.75rem;
    color: #6f6f6f;
    white-space: nowrap;
    margin-left: auto;
  }

  /* ── Table ── */
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
  .log-col-type-badge {
    width: 64px;
    flex-shrink: 0;
  }
  .log-col-ip {
    width: 140px;
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
  .log-col-action {
    width: 170px;
    flex-shrink: 0;
    text-align: right;
  }

  .log-row-main {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
    min-width: 0;
  }
  .log-row--blocked {
    background: #fff1f1;
    border-left: 3px solid #da1e28;
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
  .log-rule {
    display: block;
    font-size: 0.6875rem;
    color: #da1e28;
    font-style: italic;
    margin-top: 1px;
  }

  /* Mobile-only rows */
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
    display: flex;
    gap: 4px;
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

  @media (max-width: 671px) {
    .log-search {
      max-width: none;
    }
    .log-filters {
      flex-direction: column;
      gap: 8px;
    }
    .log-header {
      display: none;
    }
    .log-row-main,
    .log-col-action {
      display: none;
    }
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
