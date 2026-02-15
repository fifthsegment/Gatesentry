<script lang="ts">
  import {
    ComposedModal,
    ModalHeader,
    ModalBody,
    ModalFooter,
    TextInput,
    FormGroup,
    Tag,
    StructuredList,
    StructuredListHead,
    StructuredListRow,
    StructuredListCell,
    StructuredListBody,
  } from "carbon-components-svelte";
  import { createEventDispatcher } from "svelte";
  import { getBasePath } from "../../lib/navigate";

  const API_BASE = getBasePath() + "/api/devices";
  const dispatch = createEventDispatcher();

  export let device: any;
  export let open = false;

  let manualName = device?.manual_name || "";
  let owner = device?.owner || "";
  let category = device?.category || "";
  let saving = false;
  let error = "";

  function getToken(): string {
    return localStorage.getItem("jwt") || "";
  }

  async function save() {
    saving = true;
    error = "";
    try {
      const token = getToken();
      const response = await fetch(`${API_BASE}/${device.id}/name`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: manualName,
          owner: owner,
          category: category,
        }),
      });
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText);
      }
      dispatch("saved");
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  function close() {
    dispatch("close");
  }

  function formatDate(isoDate: string): string {
    if (!isoDate) return "—";
    return new Date(isoDate).toLocaleString();
  }
</script>

<ComposedModal {open} on:close={close} size="lg">
  <ModalHeader
    title="Device Details"
    label={device?.display_name || "Unknown Device"}
  />
  <ModalBody hasForm>
    {#if error}
      <div class="error-message">{error}</div>
    {/if}

    <FormGroup legendText="Assign a Name">
      <TextInput
        labelText="Display Name"
        placeholder="e.g., Vivienne's iPad"
        bind:value={manualName}
      />
      <TextInput
        labelText="Owner"
        placeholder="e.g., Vivienne, Dad"
        bind:value={owner}
        style="margin-top: 0.5rem;"
      />
      <TextInput
        labelText="Category"
        placeholder="e.g., kids, adults, iot"
        bind:value={category}
        style="margin-top: 0.5rem;"
      />
    </FormGroup>

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">Identity</h5>
    <StructuredList condensed flush>
      <StructuredListHead>
        <StructuredListRow head>
          <StructuredListCell head>Property</StructuredListCell>
          <StructuredListCell head>Value</StructuredListCell>
        </StructuredListRow>
      </StructuredListHead>
      <StructuredListBody>
        <StructuredListRow>
          <StructuredListCell>DNS Name</StructuredListCell>
          <StructuredListCell>{device?.dns_name || "—"}</StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>Hostnames</StructuredListCell>
          <StructuredListCell>
            {#if device?.hostnames?.length}
              {#each device.hostnames as h}
                <Tag size="sm" type="outline">{h}</Tag>
              {/each}
            {:else}
              —
            {/if}
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>mDNS Names</StructuredListCell>
          <StructuredListCell>
            {#if device?.mdns_names?.length}
              {#each device.mdns_names as m}
                <Tag size="sm" type="blue">{m}</Tag>
              {/each}
            {:else}
              —
            {/if}
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>IPv4</StructuredListCell>
          <StructuredListCell>{device?.ipv4 || "—"}</StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>IPv6</StructuredListCell>
          <StructuredListCell>
            <span class="ipv6-value">{device?.ipv6 || "—"}</span>
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>MAC Address(es)</StructuredListCell>
          <StructuredListCell>
            {#if device?.macs?.length}
              {#each device.macs as mac}
                <Tag size="sm" type="warm-gray">{mac}</Tag>
              {/each}
            {:else}
              —
            {/if}
          </StructuredListCell>
        </StructuredListRow>
      </StructuredListBody>
    </StructuredList>

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">Discovery</h5>
    <StructuredList condensed flush>
      <StructuredListHead>
        <StructuredListRow head>
          <StructuredListCell head>Property</StructuredListCell>
          <StructuredListCell head>Value</StructuredListCell>
        </StructuredListRow>
      </StructuredListHead>
      <StructuredListBody>
        <StructuredListRow>
          <StructuredListCell>Status</StructuredListCell>
          <StructuredListCell>
            <span class="status-dot {device?.online ? 'online' : 'offline'}"
            ></span>
            {device?.online ? "Online" : "Offline"}
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>Primary Source</StructuredListCell>
          <StructuredListCell>
            <Tag
              size="sm"
              type={device?.source === "ddns"
                ? "green"
                : device?.source === "mdns"
                ? "blue"
                : device?.source === "passive"
                ? "warm-gray"
                : device?.source === "manual"
                ? "purple"
                : "gray"}>{device?.source || "—"}</Tag
            >
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>All Sources</StructuredListCell>
          <StructuredListCell>
            {#if device?.sources?.length}
              {#each device.sources as s}
                <Tag size="sm" type="outline">{s}</Tag>
              {/each}
            {:else}
              —
            {/if}
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>First Seen</StructuredListCell>
          <StructuredListCell
            >{formatDate(device?.first_seen)}</StructuredListCell
          >
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>Last Seen</StructuredListCell>
          <StructuredListCell
            >{formatDate(device?.last_seen)}</StructuredListCell
          >
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>Device ID</StructuredListCell>
          <StructuredListCell>
            <code style="font-size: 0.75rem;">{device?.id || "—"}</code>
          </StructuredListCell>
        </StructuredListRow>
      </StructuredListBody>
    </StructuredList>
  </ModalBody>
  <ModalFooter
    primaryButtonText={saving ? "Saving..." : "Save"}
    primaryButtonDisabled={saving}
    secondaryButtonText="Cancel"
    on:click:button--primary={save}
    on:click:button--secondary={close}
  />
</ComposedModal>

<style>
  .status-dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    vertical-align: middle;
    margin-right: 0.25rem;
  }
  .status-dot.online {
    background-color: #24a148;
  }
  .status-dot.offline {
    background-color: #8d8d8d;
  }
  .error-message {
    color: #da1e28;
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }
  .ipv6-value {
    word-break: break-all;
    font-size: 0.8125rem;
  }
</style>
