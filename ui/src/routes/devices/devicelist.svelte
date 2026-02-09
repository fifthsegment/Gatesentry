<script lang="ts">
  import {
    Button,
    DataTable,
    InlineLoading,
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
  let loading = false;
  let error = "";
  let success = "";
  let selectedDevice: Device | null = null;
  let detailOpen = false;
  let refreshInterval: ReturnType<typeof setInterval>;

  const headers = [
    { key: "status", value: "Status" },
    { key: "display_name", value: "Name" },
    { key: "dns_name", value: "DNS Name" },
    { key: "ipv4", value: "IPv4" },
    { key: "ipv6", value: "IPv6" },
    { key: "macs_display", value: "MAC" },
    { key: "source", value: "Via" },
    { key: "last_seen_display", value: "Last Seen" },
    { key: "actions", value: "" },
  ];

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
        // DNS server not started yet — not an error, just empty
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
      status: d.online ? "online" : "offline",
      display_name: d.manual_name || d.display_name || d.dns_name || "Unknown",
      macs_display: d.macs?.length ? d.macs[0] : "—",
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

  onMount(() => {
    loadDevices();
    // Auto-refresh every 30 seconds
    refreshInterval = setInterval(loadDevices, 30000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });
</script>

{#if error}
  <InlineNotification
    kind="error"
    title="Error"
    subtitle={error}
    on:close={() => (error = "")}
  />
{/if}

{#if success}
  <InlineNotification
    kind="success"
    title="Success"
    subtitle={success}
    on:close={() => (success = "")}
  />
{/if}

{#if loading && devices.length === 0}
  <InlineLoading description="Loading devices..." />
{:else}
  <DataTable
    sortable
    title="Device Inventory"
    description="Devices discovered on your network via DNS queries, mDNS, and DDNS updates."
    {headers}
    rows={devices}
  >
    <Toolbar>
      <ToolbarContent>
        <ToolbarSearch
          persistent
          shouldFilterRows
          placeholder="Search devices..."
        />
        <Button
          kind="ghost"
          icon={Renew}
          iconDescription="Refresh"
          on:click={loadDevices}
        />
      </ToolbarContent>
    </Toolbar>
    <svelte:fragment slot="cell" let:row let:cell>
      {#if cell.key === "status"}
        <span
          class="status-dot {row.online ? 'online' : 'offline'}"
          title={row.online ? "Online" : "Offline"}
        ></span>
      {:else if cell.key === "display_name"}
        <button class="device-name-link" on:click={() => openDetail(row)}>
          {cell.value}
        </button>
        {#if !row.manual_name && row.source !== "manual"}
          <Tag size="sm" type="cyan">auto</Tag>
        {/if}
      {:else if cell.key === "source"}
        <Tag
          size="sm"
          type={cell.value === "ddns"
            ? "green"
            : cell.value === "mdns"
            ? "blue"
            : cell.value === "passive"
            ? "warm-gray"
            : cell.value === "manual"
            ? "purple"
            : "gray"}>{cell.value}</Tag
        >
      {:else if cell.key === "actions"}
        <OverflowMenu flipped>
          <OverflowMenuItem
            text="Edit / Name"
            on:click={() => openDetail(row)}
          />
          <OverflowMenuItem
            danger
            text="Remove"
            on:click={() => removeDevice(row)}
          />
        </OverflowMenu>
      {:else}
        {cell.value || "—"}
      {/if}
    </svelte:fragment>
  </DataTable>

  <div class="device-summary">
    {devices.length} device{devices.length !== 1 ? "s" : ""} discovered · {devices.filter(
      (d) => d.online,
    ).length} online
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
  .status-dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    vertical-align: middle;
  }
  .status-dot.online {
    background-color: #24a148; /* Carbon green-60 */
  }
  .status-dot.offline {
    background-color: #8d8d8d; /* Carbon gray-50 */
  }
  .device-name-link {
    background: none;
    border: none;
    color: #0f62fe; /* Carbon blue-60 */
    cursor: pointer;
    padding: 0;
    font: inherit;
    text-decoration: underline;
  }
  .device-name-link:hover {
    color: #0043ce; /* Carbon blue-70 */
  }
  .device-summary {
    margin-top: 1rem;
    font-size: 0.875rem;
    color: #525252; /* Carbon gray-70 */
  }
</style>
