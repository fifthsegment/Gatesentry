<script lang="ts">
  import {
    InlineLoading,
    InlineNotification,
    Search,
    Tag,
  } from "carbon-components-svelte";
  import { ChevronRight, Renew } from "carbon-icons-svelte";
  import { onMount, onDestroy } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import DeviceDetail from "./devicedetail.svelte";

  const API_BASE = getBasePath() + "/api/devices";

  interface Device {
    id: string;
    display_name: string;
    dns_name: string;
    manual_name: string;
    hostnames: string[];
    mdns_names: string[];
    macs: string[];
    ipv4: string;
    ipv6: string;
    source: string;
    sources: string[];
    first_seen: string;
    last_seen: string;
    online: boolean;
    owner: string;
    category: string;
    persistent: boolean;
  }

  let devices: Device[] = [];
  let search = "";
  let loading = false;
  let error = "";
  let success = "";
  let selectedDevice: Device | null = null;
  let detailOpen = false;
  let refreshInterval: ReturnType<typeof setInterval>;

  function getToken(): string {
    return localStorage.getItem("jwt") || "";
  }

  async function loadDevices() {
    loading = true;
    error = "";
    try {
      const token = getToken();
      if (!token) {
        throw new Error("Please login first to view devices");
      }
      const response = await fetch(API_BASE, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (response.status === 401) {
        throw new Error("Authentication failed. Please login again");
      }
      if (response.status === 503) {
        devices = [];
        return;
      }
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(
          `Failed to load devices: ${response.status} - ${errorText}`,
        );
      }
      const data = await response.json();
      devices = (data.devices || []).map(formatDevice);
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  function formatDevice(d: Device): Device & Record<string, any> {
    return {
      ...d,
      id: d.id,
      display_name: d.manual_name || d.display_name || d.dns_name || "Unknown",
      macs_display: d.macs?.length ? d.macs[0] : "",
      last_seen_display: formatTimeAgo(d.last_seen),
    };
  }

  function formatTimeAgo(isoDate: string): string {
    if (!isoDate) return "Never";
    const date = new Date(isoDate);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return "Just now";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    return `${diffDay}d ago`;
  }

  function sourceTagType(s: string): string {
    if (s === "ddns") return "green";
    if (s === "mdns") return "blue";
    if (s === "passive") return "warm-gray";
    if (s === "manual") return "purple";
    return "gray";
  }

  function openDetail(device: Device) {
    selectedDevice = device;
    detailOpen = true;
  }

  async function handleNameSaved() {
    detailOpen = false;
    selectedDevice = null;
    success = "Device updated successfully";
    setTimeout(() => (success = ""), 3000);
    await loadDevices();
  }

  async function removeDevice(device: Device) {
    if (!confirm(`Remove "${device.display_name}" from the inventory?`)) return;
    try {
      const token = getToken();
      const response = await fetch(`${API_BASE}/${device.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to remove device: ${errorText}`);
      }
      success = `"${device.display_name}" removed`;
      setTimeout(() => (success = ""), 3000);
      await loadDevices();
    } catch (err) {
      error = err.message;
    }
  }

  $: filtered = search
    ? devices.filter((d) => {
        const q = search.toLowerCase();
        return (
          d.display_name?.toLowerCase().includes(q) ||
          d.dns_name?.toLowerCase().includes(q) ||
          d.ipv4?.includes(q) ||
          d.ipv6?.includes(q) ||
          d.macs_display?.toLowerCase().includes(q) ||
          d.source?.toLowerCase().includes(q) ||
          d.owner?.toLowerCase().includes(q)
        );
      })
    : devices;

  $: onlineCount = devices.filter((d) => d.online).length;

  onMount(() => {
    loadDevices();
    refreshInterval = setInterval(loadDevices, 30000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });
</script>

{#if error}
  <div class="dl-notice">
    <InlineNotification
      kind="error"
      title="Error"
      subtitle={error}
      on:close={() => (error = "")}
    />
  </div>
{/if}

{#if success}
  <div class="dl-notice">
    <InlineNotification
      kind="success"
      title="Success"
      subtitle={success}
      on:close={() => (success = "")}
    />
  </div>
{/if}

<div class="dl-toolbar">
  <div class="dl-search">
    <Search bind:value={search} placeholder="Search devices…" size="sm" />
  </div>
  <button class="dl-refresh" on:click={loadDevices} title="Refresh">
    <Renew size={20} />
  </button>
</div>

{#if loading && devices.length === 0}
  <InlineLoading description="Loading devices…" />
{:else}
  <div class="gs-card-flush">
    <div class="gs-row-list">
      {#if filtered.length === 0}
        <div class="gs-row-item">
          <span class="gs-empty"
            >{search
              ? "No matching devices"
              : "No devices discovered yet"}</span
          >
        </div>
      {/if}
      {#each filtered as dev (dev.id)}
        <button class="gs-row-item dl-row" on:click={() => openDetail(dev)}>
          <!-- Desktop layout -->
          <div class="dl-desktop">
            <span
              class="dl-dot"
              class:dl-online={dev.online}
              class:dl-offline={!dev.online}
            ></span>
            <span class="dl-name">
              {dev.display_name}
              {#if !dev.manual_name && dev.source !== "manual"}
                <Tag size="sm" type="cyan">auto</Tag>
              {/if}
            </span>
            <span class="dl-ip">{dev.ipv4 || "—"}</span>
            <span class="dl-mac">{dev.macs_display || "—"}</span>
            <span class="dl-source"
              ><Tag size="sm" type={sourceTagType(dev.source)}>{dev.source}</Tag
              ></span
            >
            <span class="dl-seen">{dev.last_seen_display}</span>
          </div>
          <!-- Mobile layout -->
          <div class="dl-mobile">
            <div class="dl-mob-top">
              <span class="dl-mob-name">
                <span
                  class="dl-dot"
                  class:dl-online={dev.online}
                  class:dl-offline={!dev.online}
                ></span>
                {dev.display_name}
              </span>
              <Tag size="sm" type={sourceTagType(dev.source)}>{dev.source}</Tag>
            </div>
            <div class="dl-mob-mid">
              <span class="dl-ip">{dev.ipv4 || "—"}</span>
              {#if dev.macs_display}
                <span class="dl-mac-sep">·</span>
                <span class="dl-mac">{dev.macs_display}</span>
              {/if}
            </div>
            <div class="dl-mob-bottom">
              <span class="dl-seen">{dev.last_seen_display}</span>
            </div>
          </div>
          <span class="dl-chevron"><ChevronRight size={20} /></span>
        </button>
      {/each}
    </div>
  </div>

  <div
    class="gs-info-footer"
    style="border:none; background:transparent; padding: 8px 0;"
  >
    <span class="gs-info-icon">i</span>
    <p>
      {devices.length} device{devices.length !== 1 ? "s" : ""} discovered · {onlineCount}
      online
    </p>
  </div>
{/if}

{#if detailOpen && selectedDevice}
  <DeviceDetail
    device={selectedDevice}
    open={detailOpen}
    on:close={() => {
      detailOpen = false;
      selectedDevice = null;
    }}
    on:saved={handleNameSaved}
  />
{/if}

<style>
  .dl-notice {
    margin-bottom: 0.75rem;
  }
  .dl-notice :global(.bx--inline-notification) {
    max-width: 100%;
  }

  .dl-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 0.75rem;
  }
  .dl-search {
    flex: 1;
    max-width: 360px;
  }
  .dl-refresh {
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
  .dl-refresh:hover {
    background: #e5e5e5;
  }

  /* Row as a button */
  .dl-row {
    cursor: pointer;
    border: none;
    text-align: left;
    font: inherit;
    width: 100%;
  }
  .dl-row:hover {
    background: #f4f4f4 !important;
  }

  /* Status dot */
  .dl-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }
  .dl-online {
    background-color: #24a148;
  }
  .dl-offline {
    background-color: #c6c6c6;
  }

  /* Desktop row */
  .dl-desktop {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: 1;
    min-width: 0;
  }
  .dl-name {
    flex: 1;
    min-width: 0;
    font-size: 0.875rem;
    font-weight: 500;
    color: #161616;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .dl-ip {
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.8125rem;
    color: #393939;
    width: 110px;
    flex-shrink: 0;
  }
  .dl-mac {
    font-family: "IBM Plex Mono", monospace;
    font-size: 0.75rem;
    color: #6f6f6f;
    width: 130px;
    flex-shrink: 0;
  }
  .dl-source {
    width: 70px;
    flex-shrink: 0;
  }
  .dl-seen {
    font-size: 0.8125rem;
    color: #6f6f6f;
    width: 80px;
    flex-shrink: 0;
    text-align: right;
  }

  .dl-chevron {
    color: #a8a8a8;
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }

  /* Mobile layout — hidden on desktop */
  .dl-mobile {
    display: none;
  }
  .dl-mob-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    gap: 8px;
  }
  .dl-mob-name {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.875rem;
    font-weight: 500;
    color: #161616;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dl-mob-mid {
    display: flex;
    align-items: center;
    gap: 4px;
    width: 100%;
  }
  .dl-mob-mid .dl-ip {
    width: auto;
  }
  .dl-mob-mid .dl-mac {
    width: auto;
  }
  .dl-mac-sep {
    color: #a8a8a8;
  }
  .dl-mob-bottom {
    width: 100%;
  }
  .dl-mob-bottom .dl-seen {
    width: auto;
    text-align: left;
  }

  /* ── Mobile ── */
  @media (max-width: 671px) {
    .dl-search {
      max-width: none;
    }
    .dl-desktop {
      display: none;
    }
    .dl-mobile {
      display: flex;
      flex-direction: column;
      gap: 4px;
      flex: 1;
      min-width: 0;
    }
    .dl-row {
      flex-direction: row;
      align-items: center;
    }
  }
</style>
