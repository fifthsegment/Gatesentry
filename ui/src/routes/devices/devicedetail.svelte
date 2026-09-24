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
    InlineNotification,
    StructuredList,
    StructuredListHead,
    StructuredListRow,
    StructuredListCell,
    StructuredListBody,
  } from "carbon-components-svelte";
  import { createEventDispatcher, onDestroy } from "svelte";
  import { getBasePath } from "../../lib/navigate";

  const API_BASE = getBasePath() + "/api/devices";
  const POLICY_BASE = getBasePath() + "/api/policy";
  const TAILSCALE_BASE = getBasePath() + "/api/tailscale";
  const dispatch = createEventDispatcher();

  interface TailscaleSuggestion {
    device_id: string;
    name: string;
    reason: string;
    confidence: string;
  }

  interface TailscalePeerState {
    enabled: boolean;
    detected: boolean;
    state: string;
    backend_state: string;
    self_name: string;
    last_refresh: string;
    message: string;
  }

  interface TailscalePeer {
    node_id: string;
    name: string;
    dns_name: string;
    addresses: string[];
    online: boolean;
    last_seen: string;
    mapped_device_id: string;
    suggestion?: TailscaleSuggestion;
    wol_macs?: string[];
  }

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
  let tailscalePeers: TailscalePeer[] = [];
  let tailscaleState: TailscalePeerState | null = null;
  let linkedTailscaleNodes: TailscalePeer[] = device?.tailscale_nodes || [];
  let availableTailscalePeers: TailscalePeer[] = [];
  let selectedPeerNodeID = "";
  let tailscaleLoading = false;
  let tailscaleLoaded = false;
  let tailscaleSaving = false;
  let tailscaleError = "";
  let tailscaleNotice = "";
  let activity: any[] = [];
  let activityLoading = false;
  let activityError = "";
  let activityLoaded = false;
  let activityTimer: ReturnType<typeof setTimeout> | null = null;
  let activityController: AbortController | null = null;
  let activityRequestRunning = false;
  let destroyed = false;
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
      const response = await fetch(API_BASE + "/" + device.id + "/policy", {
        headers: authHeaders(false),
      });
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

  async function loadTailscalePeers() {
    tailscaleLoading = true;
    tailscaleError = "";
    try {
      const response = await fetch(TAILSCALE_BASE + "/peers", {
        headers: authHeaders(false),
      });
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      const data = await response.json();
      tailscaleState = data;
      tailscalePeers = data.state === "connected" ? data.peers || [] : [];
      if (data.state === "connected") {
        linkedTailscaleNodes = tailscalePeers.filter(
          (peer) => peer.mapped_device_id === device.id,
        );
      }
      tailscaleLoaded = true;
    } catch (err) {
      tailscaleState = null;
      tailscalePeers = [];
      tailscaleError =
        err instanceof Error ? err.message : "Tailscale peers failed";
    } finally {
      tailscaleLoading = false;
    }
  }

  async function linkTailscalePeer() {
    if (!selectedPeerNodeID) return;
    tailscaleSaving = true;
    tailscaleError = "";
    tailscaleNotice = "";
    try {
      const response = await fetch(API_BASE + "/" + device.id + "/tailscale", {
        method: "PUT",
        headers: authHeaders(true),
        body: JSON.stringify({ node_id: selectedPeerNodeID }),
      });
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      selectedPeerNodeID = "";
      tailscaleNotice = "Tailscale peer linked.";
      await loadTailscalePeers();
      dispatch("tailscaleChanged");
    } catch (err) {
      tailscaleError = err.message;
    } finally {
      tailscaleSaving = false;
    }
  }

  async function unlinkTailscalePeer(nodeID: string, name: string) {
    if (
      !confirm(`Unlink Tailscale peer "${name || nodeID}" from this device?`)
    ) {
      return;
    }
    tailscaleSaving = true;
    tailscaleError = "";
    tailscaleNotice = "";
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/tailscale/" + encodeURIComponent(nodeID),
        { method: "DELETE", headers: authHeaders(false) },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      linkedTailscaleNodes = linkedTailscaleNodes.filter(
        (peer) => peer.node_id !== nodeID,
      );
      tailscaleNotice = "Tailscale peer unlinked.";
      await loadTailscalePeers();
      dispatch("tailscaleChanged");
    } catch (err) {
      tailscaleError = err.message;
    } finally {
      tailscaleSaving = false;
    }
  }

  function tailscaleStateKind(
    state: string,
  ): "success" | "warning" | "error" | "info" {
    if (state === "connected") return "success";
    if (state === "degraded" || state === "connecting") return "warning";
    if (state === "unavailable" || state === "stopped") return "error";
    return "info";
  }

  function tailscaleStateTitle(state: string): string {
    if (state === "connected") return "Tailscale connected";
    if (state === "degraded") return "Tailscale data degraded";
    if (state === "unavailable") return "Tailscale unavailable";
    if (state === "connecting") return "Tailscale connecting";
    if (state === "disabled") return "Tailscale disabled";
    return "Tailscale status";
  }

  function peerName(peer: TailscalePeer): string {
    return peer.name || peer.dns_name || peer.node_id;
  }

  $: availableTailscalePeers = tailscalePeers.filter(
    (peer) => !peer.mapped_device_id,
  );

  async function loadActivity() {
    if (activityRequestRunning || destroyed) return;
    activityRequestRunning = true;
    activityLoading = !activityLoaded;
    activityError = "";
    activityController = new AbortController();
    try {
      const response = await fetch(
        API_BASE + "/" + device.id + "/activity?since=86400&limit=100",
        {
          headers: authHeaders(false),
          signal: activityController.signal,
        },
      );
      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }
      const data = await response.json();
      if (!destroyed) {
        activity = data.items || [];
        activityLoaded = true;
      }
    } catch (err) {
      if (!destroyed && err?.name !== "AbortError") {
        activityError = err.message;
      }
    } finally {
      activityRequestRunning = false;
      activityLoading = false;
      activityController = null;
      if (!destroyed) activityTimer = setTimeout(loadActivity, 5000);
    }
  }

  async function save() {
    saving = true;
    error = "";
    try {
      const response = await fetch(API_BASE + "/" + device.id + "/name", {
        method: "POST",
        headers: authHeaders(true),
        body: JSON.stringify({
          name: manualName,
          owner: owner,
          category: category,
        }),
      });
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
      const response = await fetch(API_BASE + "/" + device.id + "/assignment", {
        method: "PUT",
        headers: authHeaders(true),
        body: JSON.stringify({ group_id: selectedGroup }),
      });
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
    loadTailscalePeers();
    loadActivity();
  }

  onDestroy(() => {
    destroyed = true;
    if (activityTimer) clearTimeout(activityTimer);
    activityController?.abort();
  });

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

  // The policy view lists what the effective policy blocks and allows; a
  // category is named the way the policies page names it.
  function policySummary(policy: any): string {
    if (!policy) return "";
    const parts: string[] = [];
    if (policy.blocked_categories?.length) {
      parts.push(
        policy.blocked_categories.length +
          " blocked categor" +
          (policy.blocked_categories.length === 1 ? "y" : "ies"),
      );
    }
    if (policy.blocked_domains?.length) {
      parts.push(
        policy.blocked_domains.length +
          " blocked domain" +
          (policy.blocked_domains.length === 1 ? "" : "s"),
      );
    }
    if (policy.allowed_domains?.length) {
      parts.push(
        policy.allowed_domains.length +
          " allowed domain" +
          (policy.allowed_domains.length === 1 ? "" : "s"),
      );
    }
    if (policy.rule_count) {
      parts.push(
        policy.rule_count + " rule" + (policy.rule_count === 1 ? "" : "s"),
      );
    }
    if (policy.safe_search) parts.push("safe search");
    return parts.length ? parts.join(", ") : "blocks nothing of its own";
  }
</script>

<ComposedModal {open} on:close={close} on:submit={save} size="lg">
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

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">Policy</h5>
    <p class="section-note">
      A device without a policy of its own uses the default policy. Owner and
      category above never change filtering.
    </p>
    {#if assignmentError}
      <div class="error-message">{assignmentError}</div>
    {/if}
    {#if assignmentSaved}
      <div class="success-message">Policy saved.</div>
    {/if}
    <div class="assignment-row">
      <div class="assignment-select">
        <Select
          labelText="Policy"
          helperText="Applies to DNS immediately; proxy paths use it when identity resolves."
          bind:selected={selectedGroup}
          disabled={assignmentSaving}
        >
          <SelectItem value="" text="Default policy" />
          {#each groups.filter((group) => group.id !== "default") as group (group.id)}
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
        {assignmentSaving ? "Saving…" : "Save policy"}
      </Button>
    </div>

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">
      Effective protection
    </h5>
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
      {#if policyView.effective_policy}
        <div class="effective-rules">
          <span class="rules-label">{policyView.effective_policy.name}:</span>
          {policySummary(policyView.effective_policy)}
          {#if policyView.effective_policy.default}
            <div class="inapplicable-note">
              No policy of its own is assigned, so the default policy applies.
            </div>
          {/if}
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

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">Tailscale</h5>
    <p class="section-note">
      Linking is always manual. Suggestions are shown as hints only and never
      select or link a peer automatically.
    </p>
    <p class="section-note">
      Tailscale normally does not provide a hardware MAC address. Wake-on-LAN
      MAC addresses shown here are informational and are not used to match or
      link devices.
    </p>
    {#if tailscaleError}
      <div class="error-message">{tailscaleError}</div>
    {/if}
    {#if tailscaleNotice}
      <div class="success-message">{tailscaleNotice}</div>
    {/if}
    {#if tailscaleLoading && !tailscaleLoaded}
      <InlineLoading description="Loading Tailscale peers..." />
    {:else}
      {#if tailscaleState}
        <InlineNotification
          lowContrast
          hideCloseButton
          kind={tailscaleStateKind(tailscaleState.state)}
          title={tailscaleStateTitle(tailscaleState.state)}
          subtitle={tailscaleState.message || tailscaleState.backend_state}
        />
      {/if}

      {#if linkedTailscaleNodes.length}
        <div class="tailscale-peer-list linked-peers">
          {#each linkedTailscaleNodes as peer (peer.node_id)}
            <article class="tailscale-peer">
              <div class="peer-heading">
                <div>
                  <strong>{peerName(peer)}</strong>
                  <Tag size="sm" type={peer.online ? "green" : "warm-gray"}>
                    {peer.online ? "online" : "offline"}
                  </Tag>
                </div>
                <Button
                  kind="danger-tertiary"
                  size="small"
                  disabled={tailscaleSaving}
                  on:click={() =>
                    unlinkTailscalePeer(peer.node_id, peerName(peer))}
                >
                  Unlink
                </Button>
              </div>
              <div class="peer-details">
                <span>{peer.dns_name || peer.node_id}</span>
                {#if peer.addresses?.length}<span
                    >{peer.addresses.join(", ")}</span
                  >{/if}
                <span>Last seen: {formatDate(peer.last_seen)}</span>
              </div>
              {#if peer.wol_macs?.length}
                <div class="wol-macs">
                  <span>Informational WoL MAC:</span>
                  {#each peer.wol_macs as mac}
                    <Tag size="sm" type="outline">{mac}</Tag>
                  {/each}
                </div>
              {/if}
            </article>
          {/each}
        </div>
      {:else}
        <p class="section-note">
          No Tailscale peers are linked to this device.
        </p>
      {/if}

      {#if tailscaleState?.state === "connected"}
        <div class="tailscale-link-row">
          <div class="assignment-select">
            <Select
              labelText="Tailscale peer"
              helperText="Choose an unlinked peer, then confirm the link."
              bind:selected={selectedPeerNodeID}
              disabled={tailscaleSaving || tailscaleLoading}
            >
              <SelectItem value="" text="Choose a peer" />
              {#each availableTailscalePeers as peer (peer.node_id)}
                <SelectItem
                  value={peer.node_id}
                  text={`${peerName(peer)}${
                    peer.online ? " (online)" : " (offline)"
                  }`}
                />
              {/each}
            </Select>
          </div>
          <Button
            kind="primary"
            size="field"
            disabled={tailscaleSaving || !selectedPeerNodeID}
            on:click={linkTailscalePeer}
          >
            {tailscaleSaving ? "Saving…" : "Link peer"}
          </Button>
        </div>

        {#if availableTailscalePeers.length}
          <div class="tailscale-peer-list available-peers">
            {#each availableTailscalePeers as peer (peer.node_id)}
              <article class="tailscale-peer">
                <div class="peer-heading">
                  <strong>{peerName(peer)}</strong>
                  <Tag size="sm" type={peer.online ? "green" : "warm-gray"}>
                    {peer.online ? "online" : "offline"}
                  </Tag>
                </div>
                <div class="peer-details">
                  {#if peer.dns_name}<span>{peer.dns_name}</span>{/if}
                  {#if peer.addresses?.length}<span
                      >{peer.addresses.join(", ")}</span
                    >{/if}
                  <span>Last seen: {formatDate(peer.last_seen)}</span>
                </div>
                {#if peer.suggestion}
                  <p class="suggestion">
                    Suggested match: {peer.suggestion.name ||
                      peer.suggestion.device_id}
                    ({peer.suggestion.confidence || "unknown"} confidence) —
                    {peer.suggestion.reason || "No reason supplied"}. Review and
                    choose the peer above to link it.
                  </p>
                {/if}
                {#if peer.wol_macs?.length}
                  <div class="wol-macs">
                    <span>Informational WoL MAC:</span>
                    {#each peer.wol_macs as mac}
                      <Tag size="sm" type="outline">{mac}</Tag>
                    {/each}
                  </div>
                {/if}
              </article>
            {/each}
          </div>
        {:else if tailscaleLoaded}
          <p class="section-note">No unlinked Tailscale peers are available.</p>
        {/if}
      {/if}
      <div class="tailscale-refresh-row">
        <Button
          kind="tertiary"
          size="small"
          disabled={tailscaleSaving || tailscaleLoading}
          on:click={loadTailscalePeers}
        >
          Refresh Tailscale status
        </Button>
      </div>
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
              <Tag size="sm" type="blue">{policyView.identity.source}</Tag>
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
                    <Tag size="sm" type="warm-gray">stale observation</Tag>
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
                    <Tag size="sm" type="warm-gray">stale observation</Tag>
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

    <h5 style="margin-top: 1.5rem; margin-bottom: 0.5rem;">
      Recent activity (decision history)
    </h5>
    {#if activityLoading && !activityLoaded}
      <InlineLoading description="Loading device activity..." />
    {:else if activity.length === 0}
      <p class="section-note">
        No logged decisions for this device's current addresses in the last 24
        hours.
      </p>
    {:else}
      <ul class="activity-list">
        {#each activity as item}
          <li>
            <span class="activity-time">{formatActivityTime(item.time)}</span>
            <Tag size="sm" type={item.type === "dns" ? "blue" : "purple"}
              >{item.type}</Tag
            >
            {#if item.dnsResponseType || item.proxyResponseType}
              <Tag
                size="sm"
                type={(item.dnsResponseType || item.proxyResponseType) ===
                "blocked"
                  ? "red"
                  : "gray"}
              >
                {item.dnsResponseType || item.proxyResponseType}
              </Tag>
            {/if}
            <span class="activity-url">{item.url}</span>
            <span class="activity-ip">from {item.ip}</span>
          </li>
        {/each}
      </ul>
    {/if}
    {#if activityError}
      <div class="refresh-error">Live refresh unavailable: {activityError}</div>
    {/if}
    <p class="section-note">
      <a href={getBasePath() + "/logs"}>View full decision history</a>
    </p>
  </ModalBody>
  <ModalFooter
    primaryButtonText={saving ? "Saving..." : "Save"}
    primaryButtonDisabled={saving}
    secondaryButtonText="Cancel"
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
  .refresh-error {
    color: var(--cds-text-helper, #6f6f6f);
    font-size: 0.75rem;
    margin-top: 0.5rem;
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
  .tailscale-refresh-row {
    margin: 0.75rem 0;
  }
  .tailscale-link-row {
    display: flex;
    align-items: flex-end;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin: 0.75rem 0;
  }
  .tailscale-peer-list {
    display: grid;
    gap: 0.5rem;
    margin: 0.75rem 0;
  }
  .tailscale-peer {
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
    padding: 0.75rem;
  }
  .peer-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }
  .peer-heading > div {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .peer-details {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
    color: #525252;
    font-size: 0.8125rem;
    margin-top: 0.35rem;
  }
  .suggestion {
    background: var(--cds-layer-01, #f4f4f4);
    color: var(--cds-text-secondary, #525252);
    font-size: 0.8125rem;
    margin: 0.5rem 0 0;
    padding: 0.5rem;
  }
  .wol-macs {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 0.35rem;
    color: #525252;
    font-size: 0.8125rem;
    margin-top: 0.5rem;
  }
</style>
