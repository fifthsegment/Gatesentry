<script lang="ts">
  import {
    ComposedModal,
    ModalHeader,
    ModalBody,
    ModalFooter,
    TextInput,
    FormGroup,
    Select,
    SelectItem,
    Button,
    Tag,
    InlineLoading,
    StructuredList,
    StructuredListHead,
    StructuredListRow,
    StructuredListCell,
    StructuredListBody,
  } from "carbon-components-svelte";
  import { createEventDispatcher } from "svelte";
  import { getBasePath } from "../../lib/navigate";

  const API_BASE = getBasePath() + "/api/devices";
  const POLICY_BASE = getBasePath() + "/api/policy";
  const dispatch = createEventDispatcher();

  export let device: any;
  export let open = false;

  let manualName = device?.manual_name || "";
  let owner = device?.owner || "";
  let category = device?.category || "";
  let saving = false;
  let error = "";

  let policyView: any = null;
  let policyLoading = false;
  let policyError = "";
  let groups: any[] = [];
  let selectedGroup = "";
  let assignmentSaving = false;
  let assignmentError = "";
  let assignmentSaved = false;
  let activity: any[] = [];
  let activityLoading = false;
  let activityError = "";
  let detailLoadedFor = "";

  function getToken(): string {
    return localStorage.getItem("jwt") || "";
  }

  function authHeaders(json: boolean): Record<string, string> {
    const headers: Record<string, string> = {
      Authorization: "Bearer " + getToken(),
    };
    if (json) headers["Content-Type"] = "application/json";
    return headers;
  }

  async function readErrorMessage(response: Response): Promise<string> {
    const text = await response.text();
    try {
      const parsed = JSON.parse(text);
      if (parsed && parsed.error) return parsed.error;
    } catch {
      // not JSON; fall through to raw text
    }
    return text || "Request failed (" + response.status + ")";
  }

  async function loadPolicyView() {
    policyLoading = true;
    policyError = "";
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/policy",
        { headers: authHeaders(false) },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      policyView = await response.json();
      selectedGroup = policyView?.assignment?.id || "";
    } catch (err) {
      policyError = err.message;
    } finally {
      policyLoading = false;
    }
  }

  async function loadGroups() {
    try {
      const response = await fetch(POLICY_BASE + "/groups", {
        headers: authHeaders(false),
      });
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      const data = await response.json();
      groups = data.groups || [];
    } catch (err) {
      // The select still offers the default option; surface the error quietly
      // next to the policy view where it is actionable.
      policyError = policyError || err.message;
    }
  }

  async function loadActivity() {
    activityLoading = true;
    activityError = "";
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/activity",
        { headers: authHeaders(false) },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      const data = await response.json();
      activity = data.items || [];
    } catch (err) {
      activityError = err.message;
    } finally {
      activityLoading = false;
    }
  }

  async function save() {
    saving = true;
    error = "";
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/name",
        {
          method: "POST",
          headers: authHeaders(true),
          body: JSON.stringify({
            name: manualName,
            owner: owner,
            category: category,
          }),
        },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      dispatch("saved");
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function saveAssignment() {
    assignmentSaving = true;
    assignmentError = "";
    assignmentSaved = false;
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/assignment",
        {
          method: "PUT",
          headers: authHeaders(true),
          body: JSON.stringify({ group_id: selectedGroup }),
        },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      await loadPolicyView();
      assignmentSaved = true;
      dispatch("saved");
    } catch (err) {
      assignmentError = err.message;
    } finally {
      assignmentSaving = false;
    }
  }

  // The parent renders this modal inside an {#if} block, so this loads once
  // per opened device and stays stable while the modal is open.
  $: if (open && device?.id && detailLoadedFor !== device.id) {
    detailLoadedFor = device.id;
    loadPolicyView();
    loadGroups();
    loadActivity();
  }

  function close() {
    dispatch("close");
  }

  function formatDate(isoDate: string): string {
    if (!isoDate) return "—";
    return new Date(isoDate).toLocaleString();
  }

  function formatActivityTime(seconds: number): string {
    if (!seconds) return "—";
    return new Date(seconds * 1000).toLocaleString();
  }

  function confidenceTagType(
    confidence: string,
  ): "green" | "teal" | "magenta" | "red" {
    switch (confidence) {
      case "high":
        return "green";
      case "medium":
        return "teal";
      case "low":
        return "magenta";
      default:
        return "red";
    }
  }

  function confidenceLabel(confidence: string): string {
    switch (confidence) {
      case "high":
        return "High confidence";
      case "medium":
        return "Medium confidence";
      case "low":
        return "Low confidence";
      default:
        return "No protection signal";
    }
  }

  function actionLabel(action: string): string {
    switch (action) {
      case "block":
        return "Blocked domains";
      case "allow":
        return "Allowed (exempt) domains";
      default:
        return "Domain rules";
    }
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

    <FormGroup legendText="Naming and metadata">
      <TextInput
        labelText="Display Name"
        placeholder="e.g., Vivienne's iPad"
        bind:value={manualName}
      />
      <TextInput
        labelText="Owner"
        helperText="Descriptive label only — it does not change filtering."
        placeholder="e.g., Vivienne, Dad"
        bind:value={owner}
        style="margin-top: 0.5rem;"
      />
      <TextInput
        labelText="Category"
        helperText="Descriptive label only — it does not change filtering."
        placeholder="e.g., kids, adults, iot"
        bind:value={category}
        style="margin-top: 0.5rem;"
      />
    </FormGroup>

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;"
      >Policy group assignment</h5
    >
    <p class="section-note"
      >Only an explicit group assignment changes filtering for this
      device.</p
    >
    {#if assignmentError}
      <div class="error-message">{assignmentError}</div>
    {/if}
    {#if assignmentSaved}
      <div class="success-message">Assignment saved.</div>
    {/if}
    <div class="assignment-row">
      <div class="assignment-select">
        <Select
          labelText="Assigned group"
          helperText="Applies to DNS immediately; proxy paths use it when identity resolves."
          bind:selected={selectedGroup}
          disabled={assignmentSaving}
        >
          <SelectItem
            value=""
            text="No group (default policy)"
          />
          {#each groups as group (group.id)}
            <SelectItem value={group.id} text={group.name || group.id} />
          {/each}
        </Select>
      </div>
      <Button
        kind="primary"
        size="field"
        disabled={assignmentSaving}
        on:click={saveAssignment}
      >
        {assignmentSaving ? "Saving…" : "Save assignment"}
      </Button>
    </div>

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;"
      >Effective protection</h5
    >
    {#if policyLoading && !policyView}
      <InlineLoading description="Loading protection view..." />
    {:else if policyError && !policyView}
      <div class="error-message">{policyError}</div>
    {:else if policyView}
      <div class="coverage-summary">
        <Tag
          size="sm"
          type={confidenceTagType(policyView.coverage?.confidence)}
        >
          {confidenceLabel(policyView.coverage?.confidence)}
        </Tag>
        <span>{policyView.coverage?.summary}</span>
      </div>
      {#if policyView.assignment}
        <div class="effective-rules">
          <span class="rules-label"
            >{actionLabel(policyView.assignment.action)}:</span
          >
          {#each policyView.assignment.domains || [] as domain}
            <Tag size="sm" type="outline">{domain}</Tag>
          {/each}
          <div class="inapplicable-note">
            Not enforceable by DNS policy:
            {(policyView.assignment.inapplicable_conditions || []).join(", ")}
          </div>
        </div>
      {:else}
        <div class="effective-rules">
          No group is assigned — the gateway default policy applies.
        </div>
      {/if}
      <ul class="caveat-list">
        {#each policyView.coverage?.caveats || [] as caveat}
          <li>{caveat}</li>
        {/each}
      </ul>
      <p class="section-note">
        {policyView.metadata_note ||
          "Owner and category are descriptive labels only."}
      </p>
    {/if}

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
          <StructuredListCell>Policy Identity</StructuredListCell>
          <StructuredListCell>
            {#if policyView?.identity}
              <Tag size="sm" type="blue"
                >{policyView.identity.source}</Tag
              >
              {policyView.identity.explanation || ""}
            {:else}
              —
            {/if}
          </StructuredListCell>
        </StructuredListRow>
        {#if policyView?.identity?.device_id}
          <StructuredListRow>
            <StructuredListCell>Resolved Device</StructuredListCell>
            <StructuredListCell
              >{policyView.identity.device_id}</StructuredListCell
            >
          </StructuredListRow>
        {/if}
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
          <StructuredListCell>
            {device?.ipv4 || "—"}
            {#if policyView?.addresses}
              {#each policyView.addresses as address}
                {#if address.ip === device?.ipv4}
                  {#if address.shared_address}
                    <Tag size="sm" type="magenta">shared address</Tag>
                  {/if}
                  {#if address.stale_observation}
                    <Tag size="sm" type="warm-gray"
                      >stale observation</Tag
                    >
                  {/if}
                  {#if !address.dns_resolves_device}
                    <Tag size="sm" type="red"
                      >DNS does not resolve to this device</Tag
                    >
                  {/if}
                {/if}
              {/each}
            {/if}
          </StructuredListCell>
        </StructuredListRow>
        <StructuredListRow>
          <StructuredListCell>IPv6</StructuredListCell>
          <StructuredListCell>
            <span class="ipv6-value">{device?.ipv6 || "—"}</span>
            {#if policyView?.addresses}
              {#each policyView.addresses as address}
                {#if address.ip === device?.ipv6 && device?.ipv6}
                  {#if address.shared_address}
                    <Tag size="sm" type="magenta">shared address</Tag>
                  {/if}
                  {#if address.stale_observation}
                    <Tag size="sm" type="warm-gray"
                      >stale observation</Tag
                    >
                  {/if}
                  {#if !address.dns_resolves_device}
                    <Tag size="sm" type="red"
                      >DNS does not resolve to this device</Tag
                    >
                  {/if}
                {/if}
              {/each}
            {/if}
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
            <span
              class="status-dot {device?.online ? 'online' : 'offline'}"
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

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;"
      >Recent activity (decision history)</h5
    >
    {#if activityLoading}
      <InlineLoading description="Loading device activity..." />
    {:else if activityError}
      <div class="error-message">{activityError}</div>
    {:else if activity.length === 0}
      <p class="section-note">
        No logged decisions for this device's current addresses in the last 24
        hours.
      </p>
    {:else}
      <ul class="activity-list">
        {#each activity as item}
          <li>
            <span class="activity-time"
              >{formatActivityTime(item.time)}</span
            >
            <Tag size="sm" type={item.type === "dns" ? "blue" : "purple"}
              >{item.type}</Tag
            >
            {#if item.dnsResponseType || item.proxyResponseType}
              <Tag size="sm" type={
                (item.dnsResponseType || item.proxyResponseType) === "blocked"
                  ? "red"
                  : "gray"
              }>
                {item.dnsResponseType || item.proxyResponseType}
              </Tag>
            {/if}
            <span class="activity-url">{item.url}</span>
            <span class="activity-ip">from {item.ip}</span>
          </li>
        {/each}
      </ul>
    {/if}
    <p class="section-note">
      <a href={getBasePath() + "/logs"}>View full decision history</a>
    </p>
  </ModalBody>
  <ModalFooter
    primaryButtonText={saving ? "Saving..." : "Save"}
    primaryButtonDisabled={saving}
    secondaryButtonText="Cancel"
    on:click:button--primary={save}
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
  .success-message {
    color: #0e6027;
    margin-bottom: 0.5rem;
    font-size: 0.875rem;
  }
  .ipv6-value {
    word-break: break-all;
    font-size: 0.8125rem;
  }
  .section-note {
    color: #525252;
    font-size: 0.8125rem;
    margin: 0.25rem 0 0.75rem;
  }
  .assignment-row {
    display: flex;
    align-items: flex-end;
    gap: 0.75rem;
  }
  .assignment-select {
    flex: 1;
  }
  .coverage-summary {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
    font-size: 0.875rem;
  }
  .effective-rules {
    margin-bottom: 0.5rem;
    font-size: 0.875rem;
  }
  .rules-label {
    margin-right: 0.5rem;
  }
  .inapplicable-note {
    color: #525252;
    font-size: 0.8125rem;
    margin-top: 0.25rem;
  }
  .caveat-list {
    margin: 0.5rem 0 0.75rem;
    padding-left: 1.25rem;
    color: #525252;
    font-size: 0.8125rem;
  }
  .caveat-list li {
    margin-bottom: 0.25rem;
  }
  .activity-list {
    list-style: none;
    margin: 0 0 0.5rem;
    padding: 0;
    font-size: 0.875rem;
  }
  .activity-list li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding: 0.35rem 0;
    border-bottom: 1px solid #e0e0e0;
  }
  .activity-list li:last-child {
    border-bottom: none;
  }
  .activity-time {
    color: #525252;
    font-size: 0.8125rem;
    white-space: nowrap;
  }
  .activity-url {
    word-break: break-all;
  }
  .activity-ip {
    color: #525252;
    font-size: 0.8125rem;
    white-space: nowrap;
  }
</style>
