<script lang="ts">
  import {
    Button,
    DataTable,
    InlineNotification,
    OverflowMenu,
    OverflowMenuItem,
    Tag,
    Toolbar,
    ToolbarContent,
    ToolbarSearch,
  } from "carbon-components-svelte";
  import { Renew } from "carbon-icons-svelte";
  import { onMount, onDestroy } from "svelte";
  import ConfirmDialog from "../../components/ConfirmDialog.svelte";
  import ResourceState from "../../components/layout/ResourceState.svelte";
  import SectionPanel from "../../components/layout/SectionPanel.svelte";
  import { getBasePath } from "../../lib/navigate";
  import DeviceDetail from "./devicedetail.svelte";

  const API_BASE = getBasePath() + "/api/devices";

  interface TailscaleNode {
    node_id: string;
    name: string;
    dns_name: string;
    addresses: string[];
    online: boolean;
    last_seen: string;
    wol_macs?: string[];
  }

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
    tailscale_nodes: TailscaleNode[];
  }

  type DeviceRow = Device & {
    status: string;
    tailscale_display: string;
    address_display: string;
    last_seen_display: string;
    [key: string]: unknown;
  };

  let devices: DeviceRow[] = [];
  let loading = false;
  let loaded = false;
  let error = "";
  let success = "";
  let selectedDevice: DeviceRow | null = null;
  let detailOpen = false;
  let pendingRemoval: DeviceRow | null = null;
  let removing = false;
  let refreshInterval: ReturnType<typeof setInterval>;
  let noticeTimer: ReturnType<typeof setTimeout> | null = null;

  const headers = [
    { key: "status", value: "Status" },
    { key: "display_name", value: "Device" },
    { key: "address_display", value: "Address" },
    { key: "tailscale_display", value: "Tailscale" },
    { key: "last_seen_display", value: "Last seen" },
    { key: "actions", value: "" },
  ];

  function getToken(): string {
    return localStorage.getItem("jwt") || "";
  }

  function showSuccess(message: string) {
    success = message;
    if (noticeTimer) clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (success = ""), 3000);
  }

  async function loadDevices() {
    loading = true;
    error = "";
    try {
      const token = getToken();
      if (!token) throw new Error("Please login first to view devices");
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
      error =
        err instanceof Error ? err.message : "Devices could not be loaded";
    } finally {
      loading = false;
      loaded = true;
    }
  }

  function formatDevice(d: Device): DeviceRow {
    return {
      ...d,
      id: d.id,
      status: d.online ? "online" : "offline",
      display_name: d.manual_name || d.display_name || d.dns_name || "Unknown",
      address_display: d.ipv4 || d.ipv6 || "—",
      tailscale_display: d.tailscale_nodes?.length
        ? d.tailscale_nodes.map((node) => node.name || node.dns_name).join(", ")
        : "—",
      last_seen_display: formatTimeAgo(d.last_seen),
    };
  }

  function formatTimeAgo(isoDate: string): string {
    if (!isoDate) return "Never";
    const date = new Date(isoDate);
    const diffSec = Math.floor((Date.now() - date.getTime()) / 1000);
    if (diffSec < 60) return "Just now";
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${Math.floor(diffHr / 24)}d ago`;
  }

  function openDetail(device: DeviceRow) {
    selectedDevice = device;
    detailOpen = true;
  }

  function openDetailRow(row: unknown) {
    openDetail(row as DeviceRow);
  }

  function closeDetail() {
    detailOpen = false;
    selectedDevice = null;
  }

  async function handleNameSaved() {
    closeDetail();
    showSuccess("Device updated successfully");
    await loadDevices();
  }

  function requestRemoval(row: unknown) {
    pendingRemoval = row as DeviceRow;
  }

  async function confirmRemoval() {
    if (!pendingRemoval || removing) return;
    const device = pendingRemoval;
    removing = true;
    error = "";
    try {
      const response = await fetch(`${API_BASE}/${device.id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${getToken()}` },
      });
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to remove device: ${errorText}`);
      }
      pendingRemoval = null;
      showSuccess(`"${device.display_name}" removed`);
      await loadDevices();
    } catch (err) {
      error =
        err instanceof Error ? err.message : "Device could not be removed";
    } finally {
      removing = false;
    }
  }

  onMount(() => {
    loadDevices();
    refreshInterval = setInterval(loadDevices, 30000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
    if (noticeTimer) clearTimeout(noticeTimer);
  });
</script>

{#if error && devices.length > 0}
  <InlineNotification
    kind="error"
    title="Device action failed"
    subtitle={error}
    on:close={() => (error = "")}
  />
{/if}

{#if success}
  <InlineNotification
    kind="success"
    title="Device inventory updated"
    subtitle={success}
    on:close={() => (success = "")}
  />
{/if}

<SectionPanel
  title="Device inventory"
  description="Devices discovered on your network, including peers you explicitly linked from Tailscale."
>
  <svelte:fragment slot="actions">
    <Button
      kind="ghost"
      size="small"
      icon={Renew}
      disabled={loading}
      on:click={loadDevices}
    >
      {loading ? "Refreshing…" : "Refresh"}
    </Button>
  </svelte:fragment>

  {#if loading && !loaded}
    <ResourceState state="loading" message="Loading network devices…" />
  {:else if error && devices.length === 0}
    <ResourceState
      state="error"
      title="Unable to load network devices"
      message={error}
      retryLabel="Retry"
      onRetry={loadDevices}
    />
  {:else if devices.length === 0}
    <ResourceState
      state="empty"
      title="No devices discovered yet"
      message="GateSentry will list devices after it observes DNS or proxy traffic."
    />
  {:else}
    <div class="inventory-table" aria-label="Discovered device inventory">
      <DataTable sortable size="compact" {headers} rows={devices}>
        <Toolbar>
          <ToolbarContent>
            <ToolbarSearch
              persistent
              shouldFilterRows
              placeholder="Search devices…"
            />
          </ToolbarContent>
        </Toolbar>
        <svelte:fragment slot="cell" let:row let:cell>
          {#if cell.key === "status"}
            <Tag size="sm" type={row.online ? "green" : "warm-gray"}>
              {row.online ? "Online" : "Offline"}
            </Tag>
          {:else if cell.key === "display_name"}
            <button
              class="device-name-link"
              on:click={() => openDetailRow(row)}
            >
              {cell.value}
            </button>
            {#if !row.manual_name && row.source !== "manual"}
              <Tag size="sm" type="cyan">Automatic</Tag>
            {/if}
            {#if row.owner || row.category}
              <span class="device-meta">
                {[row.owner, row.category].filter(Boolean).join(" · ")}
              </span>
            {/if}
          {:else if cell.key === "tailscale_display"}
            {#if row.tailscale_nodes?.length}
              <div class="tailscale-links">
                {#each row.tailscale_nodes as node}
                  <Tag size="sm" type={node.online ? "green" : "warm-gray"}>
                    {node.name || node.dns_name || node.node_id}
                  </Tag>
                {/each}
              </div>
            {:else}
              —
            {/if}
          {:else if cell.key === "actions"}
            <OverflowMenu flipped aria-label="Device actions">
              <OverflowMenuItem
                text="View details"
                on:click={() => openDetailRow(row)}
              />
              <OverflowMenuItem
                text="Manage Tailscale links"
                on:click={() => openDetailRow(row)}
              />
              <OverflowMenuItem
                danger
                text="Remove"
                on:click={() => requestRemoval(row)}
              />
            </OverflowMenu>
          {:else}
            {cell.value || "—"}
          {/if}
        </svelte:fragment>
      </DataTable>
    </div>

    <p class="device-summary" aria-live="polite">
      {devices.length} device{devices.length !== 1 ? "s" : ""} discovered · {devices.filter(
        (device) => device.online,
      ).length} online
    </p>
  {/if}
</SectionPanel>

{#if detailOpen && selectedDevice}
  <DeviceDetail
    device={selectedDevice}
    open={detailOpen}
    on:close={closeDetail}
    on:saved={handleNameSaved}
    on:tailscaleChanged={loadDevices}
  />
{/if}

<ConfirmDialog
  open={Boolean(pendingRemoval)}
  title="Remove device?"
  label="Confirm inventory change"
  confirmText="Remove device"
  danger
  busy={removing}
  on:close={() => {
    if (!removing) pendingRemoval = null;
  }}
  on:submit={confirmRemoval}
>
  <p>
    Remove <strong>{pendingRemoval?.display_name || "this device"}</strong> from
    the inventory? GateSentry may discover it again when it next sends traffic.
  </p>
</ConfirmDialog>

<style>
  .inventory-table {
    max-width: 100%;
    min-width: 0;
    overflow-x: auto;
  }

  .inventory-table :global(.bx--data-table-container) {
    min-width: 42rem;
  }

  .device-name-link {
    margin-right: 0.5rem;
    padding: 0;
    border: 0;
    background: none;
    color: var(--cds-link-primary, #0f62fe);
    cursor: pointer;
    font: inherit;
    text-align: left;
    text-decoration: underline;
  }

  .device-name-link:hover {
    color: var(--cds-link-primary-hover, #0043ce);
  }

  .device-meta {
    display: block;
    margin-top: 0.25rem;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.75rem;
  }

  .tailscale-links {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem;
  }

  .device-summary {
    margin: 1rem 0 0;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
  }
</style>
