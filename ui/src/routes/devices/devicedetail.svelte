<script lang="ts">
  import {
    Button,
    ComposedModal,
    FormGroup,
    InlineLoading,
    InlineNotification,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Select,
    SelectItem,
    StructuredList,
    StructuredListBody,
    StructuredListCell,
    StructuredListHead,
    StructuredListRow,
    Tab,
    TabContent,
    Tabs,
    Tag,
    TextInput,
  } from "carbon-components-svelte";
  import { createEventDispatcher, onDestroy } from "svelte";
  import ConfirmDialog from "../../components/ConfirmDialog.svelte";
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
  let selectedTab = 0;

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
  let pendingUnlink: TailscalePeer | null = null;

  let activity: any[] = [];
  let activityLoading = false;
  let activityError = "";
  let activityLoaded = false;
  let activityTimer: ReturnType<typeof setTimeout> | null = null;
  let activityController: AbortController | null = null;
  let activityRequestRunning = false;
  let activityGeneration = 0;
  let destroyed = false;
  let detailLoadedFor = "";
  let activityDeviceID = "";

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
      if (parsed?.error) return parsed.error;
    } catch {
      // The backend may return plain text for older endpoints.
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
      if (!response.ok) throw new Error(await readErrorMessage(response));
      policyView = await response.json();
      selectedGroup = policyView?.assignment?.id || "";
    } catch (err) {
      policyError =
        err instanceof Error ? err.message : "Policy could not be loaded";
    } finally {
      policyLoading = false;
    }
  }

  async function loadGroups() {
    try {
      const response = await fetch(POLICY_BASE + "/groups", {
        headers: authHeaders(false),
      });
      if (!response.ok) throw new Error(await readErrorMessage(response));
      const data = await response.json();
      groups = data.groups || [];
    } catch (err) {
      policyError =
        policyError ||
        (err instanceof Error ? err.message : "Policies could not be loaded");
    }
  }

  async function loadTailscalePeers() {
    tailscaleLoading = true;
    tailscaleError = "";
    try {
      const response = await fetch(TAILSCALE_BASE + "/peers", {
        headers: authHeaders(false),
      });
      if (!response.ok) throw new Error(await readErrorMessage(response));
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
    if (!selectedPeerNodeID || tailscaleSaving) return;
    tailscaleSaving = true;
    tailscaleError = "";
    tailscaleNotice = "";
    try {
      const response = await fetch(API_BASE + "/" + device.id + "/tailscale", {
        method: "PUT",
        headers: authHeaders(true),
        body: JSON.stringify({ node_id: selectedPeerNodeID }),
      });
      if (!response.ok) throw new Error(await readErrorMessage(response));
      selectedPeerNodeID = "";
      tailscaleNotice = "Tailscale peer linked.";
      await loadTailscalePeers();
      dispatch("tailscaleChanged");
    } catch (err) {
      tailscaleError =
        err instanceof Error ? err.message : "Peer could not be linked";
    } finally {
      tailscaleSaving = false;
    }
  }

  function requestUnlink(peer: TailscalePeer) {
    pendingUnlink = peer;
  }

  async function confirmUnlink() {
    if (!pendingUnlink || tailscaleSaving) return;
    const peer = pendingUnlink;
    tailscaleSaving = true;
    tailscaleError = "";
    tailscaleNotice = "";
    try {
      const response = await fetch(
        API_BASE +
          "/" +
          device.id +
          "/tailscale/" +
          encodeURIComponent(peer.node_id),
        { method: "DELETE", headers: authHeaders(false) },
      );
      if (!response.ok) throw new Error(await readErrorMessage(response));
      linkedTailscaleNodes = linkedTailscaleNodes.filter(
        (node) => node.node_id !== peer.node_id,
      );
      pendingUnlink = null;
      tailscaleNotice = "Tailscale peer unlinked.";
      await loadTailscalePeers();
      dispatch("tailscaleChanged");
    } catch (err) {
      tailscaleError =
        err instanceof Error ? err.message : "Peer could not be unlinked";
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

  function stopActivity(resetDevice = false) {
    activityGeneration += 1;
    if (activityTimer) clearTimeout(activityTimer);
    activityTimer = null;
    activityController?.abort();
    activityController = null;
    activityRequestRunning = false;
    if (resetDevice) activityDeviceID = "";
  }

  async function loadActivity() {
    const requestedDeviceID = activityDeviceID;
    const requestGeneration = activityGeneration;
    if (
      activityRequestRunning ||
      destroyed ||
      !open ||
      !requestedDeviceID ||
      requestedDeviceID !== device?.id
    ) {
      return;
    }
    activityRequestRunning = true;
    activityLoading = !activityLoaded;
    activityError = "";
    activityController = new AbortController();
    try {
      const response = await fetch(
        API_BASE + "/" + requestedDeviceID + "/activity?since=86400&limit=100",
        { headers: authHeaders(false), signal: activityController.signal },
      );
      if (!response.ok) throw new Error(await readErrorMessage(response));
      const data = await response.json();
      if (
        !destroyed &&
        open &&
        requestGeneration === activityGeneration &&
        requestedDeviceID === activityDeviceID
      ) {
        activity = data.items || [];
        activityLoaded = true;
      }
    } catch (err) {
      if (
        !destroyed &&
        open &&
        requestGeneration === activityGeneration &&
        requestedDeviceID === activityDeviceID &&
        (err as { name?: string })?.name !== "AbortError"
      ) {
        activityError =
          err instanceof Error
            ? err.message
            : "Activity could not be refreshed";
      }
    } finally {
      if (requestGeneration === activityGeneration) {
        activityRequestRunning = false;
        activityLoading = false;
        activityController = null;
      }
      if (
        !destroyed &&
        open &&
        requestGeneration === activityGeneration &&
        requestedDeviceID === activityDeviceID
      ) {
        activityTimer = setTimeout(loadActivity, 5000);
      }
    }
  }

  function startActivity(deviceID: string) {
    stopActivity();
    activityDeviceID = deviceID;
    activity = [];
    activityLoaded = false;
    activityError = "";
    loadActivity();
  }

  async function save() {
    if (saving) return;
    saving = true;
    error = "";
    try {
      const response = await fetch(API_BASE + "/" + device.id + "/name", {
        method: "POST",
        headers: authHeaders(true),
        body: JSON.stringify({ name: manualName, owner, category }),
      });
      if (!response.ok) throw new Error(await readErrorMessage(response));
      dispatch("saved");
    } catch (err) {
      error = err instanceof Error ? err.message : "Labels could not be saved";
    } finally {
      saving = false;
    }
  }

  async function saveAssignment() {
    if (assignmentSaving) return;
    assignmentSaving = true;
    assignmentError = "";
    assignmentSaved = false;
    try {
      const response = await fetch(API_BASE + "/" + device.id + "/assignment", {
        method: "PUT",
        headers: authHeaders(true),
        body: JSON.stringify({ group_id: selectedGroup }),
      });
      if (!response.ok) throw new Error(await readErrorMessage(response));
      await loadPolicyView();
      assignmentSaved = true;
    } catch (err) {
      assignmentError =
        err instanceof Error
          ? err.message
          : "Policy assignment could not be saved";
    } finally {
      assignmentSaving = false;
    }
  }

  $: if (open && device?.id && detailLoadedFor !== device.id) {
    detailLoadedFor = device.id;
    manualName = device?.manual_name || "";
    owner = device?.owner || "";
    category = device?.category || "";
    linkedTailscaleNodes = device?.tailscale_nodes || [];
    loadPolicyView();
    loadGroups();
    loadTailscalePeers();
    startActivity(device.id);
  }

  $: if (!open && activityDeviceID) stopActivity(true);

  onDestroy(() => {
    destroyed = true;
    stopActivity(true);
  });

  function close() {
    stopActivity(true);
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
    if (confidence === "high") return "green";
    if (confidence === "medium") return "teal";
    if (confidence === "low") return "magenta";
    return "red";
  }

  function confidenceLabel(confidence: string): string {
    if (confidence === "high") return "High confidence";
    if (confidence === "medium") return "Medium confidence";
    if (confidence === "low") return "Low confidence";
    return "No protection signal";
  }

  function policySummary(policy: any): string {
    if (!policy) return "";
    const parts: string[] = [];
    if (policy.blocked_categories?.length) {
      parts.push(
        `${policy.blocked_categories.length} blocked categor${
          policy.blocked_categories.length === 1 ? "y" : "ies"
        }`,
      );
    }
    if (policy.blocked_domains?.length) {
      parts.push(
        `${policy.blocked_domains.length} blocked domain${
          policy.blocked_domains.length === 1 ? "" : "s"
        }`,
      );
    }
    if (policy.allowed_domains?.length) {
      parts.push(
        `${policy.allowed_domains.length} allowed domain${
          policy.allowed_domains.length === 1 ? "" : "s"
        }`,
      );
    }
    if (policy.rule_count) {
      parts.push(
        `${policy.rule_count} rule${policy.rule_count === 1 ? "" : "s"}`,
      );
    }
    if (policy.safe_search) parts.push("safe search");
    return parts.length ? parts.join(", ") : "blocks nothing of its own";
  }
</script>

<ComposedModal {open} on:close={close} on:submit={save} size="lg">
  <ModalHeader
    title="Device details"
    label={device?.display_name || "Unknown device"}
  />
  <ModalBody hasForm>
    {#if error}
      <InlineNotification
        kind="error"
        title="Unable to save device labels"
        subtitle={error}
        on:close={() => (error = "")}
      />
    {/if}

    <Tabs
      type="container"
      autoWidth
      bind:selected={selectedTab}
      aria-label="Device details"
    >
      <Tab label="Overview" />
      <Tab label="Policy" />
      <Tab label="Connections" />
      <Tab label="Identity & activity" />

      <svelte:fragment slot="content">
        <TabContent>
          <section class="tab-section" aria-labelledby="device-overview-title">
            <div class="section-heading">
              <h3 id="device-overview-title">Overview</h3>
              <Tag size="sm" type={device?.online ? "green" : "warm-gray"}>
                {device?.online ? "Online" : "Offline"}
              </Tag>
            </div>
            <p class="section-note">
              Add labels that make this device easier to recognize. These fields
              are descriptive and do not select protection.
            </p>
            <FormGroup legendText="Naming and metadata">
              <div class="field-grid">
                <TextInput
                  labelText="Display name"
                  placeholder="e.g., Vivienne's iPad"
                  bind:value={manualName}
                />
                <TextInput
                  labelText="Owner"
                  helperText="Descriptive label only — it does not change filtering."
                  placeholder="e.g., Vivienne, Dad"
                  bind:value={owner}
                />
                <TextInput
                  labelText="Category"
                  helperText="Descriptive label only — it does not change filtering."
                  placeholder="e.g., kids, adults, iot"
                  bind:value={category}
                />
              </div>
            </FormGroup>

            <StructuredList condensed flush>
              <StructuredListHead>
                <StructuredListRow head>
                  <StructuredListCell head>Discovery</StructuredListCell>
                  <StructuredListCell head>Observed value</StructuredListCell>
                </StructuredListRow>
              </StructuredListHead>
              <StructuredListBody>
                <StructuredListRow>
                  <StructuredListCell>Primary source</StructuredListCell>
                  <StructuredListCell>
                    <Tag size="sm" type="outline"
                      >{device?.source || "Unknown"}</Tag
                    >
                  </StructuredListCell>
                </StructuredListRow>
                <StructuredListRow>
                  <StructuredListCell>First seen</StructuredListCell>
                  <StructuredListCell
                    >{formatDate(device?.first_seen)}</StructuredListCell
                  >
                </StructuredListRow>
                <StructuredListRow>
                  <StructuredListCell>Last seen</StructuredListCell>
                  <StructuredListCell
                    >{formatDate(device?.last_seen)}</StructuredListCell
                  >
                </StructuredListRow>
              </StructuredListBody>
            </StructuredList>
          </section>
        </TabContent>

        <TabContent>
          <section class="tab-section" aria-labelledby="device-policy-title">
            <h3 id="device-policy-title">Policy</h3>
            <p class="section-note">
              A device without a policy of its own uses the default policy.
              Owner and category are metadata and never change filtering.
            </p>
            {#if assignmentError}
              <InlineNotification
                kind="error"
                lowContrast
                hideCloseButton
                title="Unable to save policy"
                subtitle={assignmentError}
              />
            {/if}
            {#if assignmentSaved}
              <InlineNotification
                kind="success"
                lowContrast
                hideCloseButton
                title="Policy saved"
                subtitle="The selected policy now applies to this device identity."
              />
            {/if}
            <div class="action-row">
              <div class="action-field">
                <Select
                  labelText="Policy"
                  helperText="Applies to DNS immediately; proxy paths use it when identity resolves."
                  bind:selected={selectedGroup}
                  disabled={assignmentSaving}
                >
                  <SelectItem value="" text="Default policy" />
                  {#each groups.filter((group) => group.id !== "default") as group (group.id)}
                    <SelectItem
                      value={group.id}
                      text={group.name || group.id}
                    />
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

            <h4>Effective protection</h4>
            {#if policyLoading && !policyView}
              <InlineLoading description="Loading protection view…" />
            {:else if policyError && !policyView}
              <InlineNotification
                kind="error"
                lowContrast
                hideCloseButton
                title="Protection view unavailable"
                subtitle={policyError}
              />
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
                <p class="effective-rules">
                  <strong>{policyView.effective_policy.name}:</strong>
                  {policySummary(policyView.effective_policy)}
                </p>
                {#if policyView.effective_policy.default}
                  <p class="section-note">
                    No policy of its own is assigned, so the default policy
                    applies.
                  </p>
                {/if}
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
          </section>
        </TabContent>

        <TabContent>
          <section
            class="tab-section"
            aria-labelledby="device-connections-title"
          >
            <h3 id="device-connections-title">Connections</h3>
            <p class="section-note">
              Linking is always manual. Suggestions are shown as hints only and
              never select or link a peer automatically.
            </p>
            <p class="section-note">
              Tailscale normally does not provide a hardware MAC address.
              Wake-on-LAN MAC addresses shown here are informational and are not
              used to match or link devices.
            </p>
            {#if tailscaleError}
              <InlineNotification
                kind="error"
                lowContrast
                hideCloseButton
                title="Tailscale action failed"
                subtitle={tailscaleError}
              />
            {/if}
            {#if tailscaleNotice}
              <InlineNotification
                kind="success"
                lowContrast
                hideCloseButton
                title="Tailscale links updated"
                subtitle={tailscaleNotice}
              />
            {/if}
            {#if tailscaleLoading && !tailscaleLoaded}
              <InlineLoading description="Loading Tailscale peers…" />
            {:else}
              {#if tailscaleState}
                <InlineNotification
                  lowContrast
                  hideCloseButton
                  kind={tailscaleStateKind(tailscaleState.state)}
                  title={tailscaleStateTitle(tailscaleState.state)}
                  subtitle={tailscaleState.message ||
                    tailscaleState.backend_state}
                />
              {/if}

              <h4>Linked peers</h4>
              {#if linkedTailscaleNodes.length}
                <div class="peer-list">
                  {#each linkedTailscaleNodes as peer (peer.node_id)}
                    <article class="peer-card">
                      <div class="peer-heading">
                        <div>
                          <strong>{peerName(peer)}</strong>
                          <Tag
                            size="sm"
                            type={peer.online ? "green" : "warm-gray"}
                          >
                            {peer.online ? "Online" : "Offline"}
                          </Tag>
                        </div>
                        <Button
                          kind="danger-tertiary"
                          size="small"
                          disabled={tailscaleSaving}
                          on:click={() => requestUnlink(peer)}>Unlink</Button
                        >
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
                          {#each peer.wol_macs as mac}<Tag
                              size="sm"
                              type="outline">{mac}</Tag
                            >{/each}
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
                <h4>Link a peer</h4>
                <div class="action-row">
                  <div class="action-field">
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
                    >{tailscaleSaving ? "Saving…" : "Link peer"}</Button
                  >
                </div>

                {#if availableTailscalePeers.length}
                  <div class="peer-list">
                    {#each availableTailscalePeers as peer (peer.node_id)}
                      <article class="peer-card">
                        <div class="peer-heading">
                          <strong>{peerName(peer)}</strong>
                          <Tag
                            size="sm"
                            type={peer.online ? "green" : "warm-gray"}
                          >
                            {peer.online ? "Online" : "Offline"}
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
                            ({peer.suggestion.confidence || "unknown"} confidence)
                            —
                            {peer.suggestion.reason || "No reason supplied"}.
                            Review and choose the peer above to link it.
                          </p>
                        {/if}
                      </article>
                    {/each}
                  </div>
                {:else if tailscaleLoaded}
                  <p class="section-note">
                    No unlinked Tailscale peers are available.
                  </p>
                {/if}
              {/if}
              <Button
                kind="tertiary"
                size="small"
                disabled={tailscaleSaving || tailscaleLoading}
                on:click={loadTailscalePeers}>Refresh Tailscale status</Button
              >
            {/if}
          </section>
        </TabContent>

        <TabContent>
          <section class="tab-section" aria-labelledby="device-identity-title">
            <h3 id="device-identity-title">Identity & activity</h3>
            <p class="section-note">
              These observed identifiers explain how GateSentry recognizes this
              device. Shared address and stale observation signals can reduce
              confidence in device-specific protection.
            </p>
            <div class="identity-list">
              <StructuredList condensed flush>
                <StructuredListHead>
                  <StructuredListRow head>
                    <StructuredListCell head>Property</StructuredListCell>
                    <StructuredListCell head>Observed value</StructuredListCell>
                  </StructuredListRow>
                </StructuredListHead>
                <StructuredListBody>
                  <StructuredListRow>
                    <StructuredListCell>Policy identity</StructuredListCell>
                    <StructuredListCell>
                      {#if policyView?.identity}
                        <Tag size="sm" type="blue"
                          >{policyView.identity.source}</Tag
                        >
                        {policyView.identity.explanation || ""}
                      {:else}—{/if}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>DNS name</StructuredListCell>
                    <StructuredListCell
                      >{device?.dns_name || "—"}</StructuredListCell
                    >
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>IPv4</StructuredListCell>
                    <StructuredListCell>
                      {device?.ipv4 || "—"}
                      {#each policyView?.addresses || [] as address}
                        {#if address.ip === device?.ipv4}
                          {#if address.shared_address}<Tag
                              size="sm"
                              type="magenta">shared address</Tag
                            >{/if}
                          {#if address.stale_observation}<Tag
                              size="sm"
                              type="warm-gray">stale observation</Tag
                            >{/if}
                          {#if !address.dns_resolves_device}<Tag
                              size="sm"
                              type="red"
                              >DNS does not resolve to this device</Tag
                            >{/if}
                        {/if}
                      {/each}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>IPv6</StructuredListCell>
                    <StructuredListCell>
                      <span class="break-value">{device?.ipv6 || "—"}</span>
                      {#each policyView?.addresses || [] as address}
                        {#if address.ip === device?.ipv6 && device?.ipv6}
                          {#if address.shared_address}<Tag
                              size="sm"
                              type="magenta">shared address</Tag
                            >{/if}
                          {#if address.stale_observation}<Tag
                              size="sm"
                              type="warm-gray">stale observation</Tag
                            >{/if}
                        {/if}
                      {/each}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>Hostnames</StructuredListCell>
                    <StructuredListCell>
                      {#if device?.hostnames?.length}
                        {#each device.hostnames as hostname}<Tag
                            size="sm"
                            type="outline">{hostname}</Tag
                          >{/each}
                      {:else}—{/if}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>mDNS names</StructuredListCell>
                    <StructuredListCell>
                      {#if device?.mdns_names?.length}
                        {#each device.mdns_names as name}<Tag
                            size="sm"
                            type="blue">{name}</Tag
                          >{/each}
                      {:else}—{/if}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>MAC addresses</StructuredListCell>
                    <StructuredListCell>
                      {#if device?.macs?.length}
                        {#each device.macs as mac}<Tag
                            size="sm"
                            type="warm-gray">{mac}</Tag
                          >{/each}
                      {:else}—{/if}
                    </StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell>Device ID</StructuredListCell>
                    <StructuredListCell
                      ><code class="break-value">{device?.id || "—"}</code
                      ></StructuredListCell
                    >
                  </StructuredListRow>
                </StructuredListBody>
              </StructuredList>
            </div>

            <h4>Recent activity (decision history)</h4>
            {#if activityLoading && !activityLoaded}
              <InlineLoading description="Loading device activity…" />
            {:else if activity.length === 0}
              <p class="section-note">
                No logged decisions for this device's current addresses in the
                last 24 hours.
              </p>
            {:else}
              <ul class="activity-list">
                {#each activity as item}
                  <li>
                    <span class="activity-time"
                      >{formatActivityTime(item.time)}</span
                    >
                    <Tag
                      size="sm"
                      type={item.type === "dns" ? "blue" : "purple"}
                      >{item.type}</Tag
                    >
                    {#if item.dnsResponseType || item.proxyResponseType}
                      <Tag
                        size="sm"
                        type={(item.dnsResponseType ||
                          item.proxyResponseType) === "blocked"
                          ? "red"
                          : "gray"}
                        >{item.dnsResponseType || item.proxyResponseType}</Tag
                      >
                    {/if}
                    <span class="activity-url">{item.url}</span>
                    <span class="activity-ip">from {item.ip}</span>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if activityError}
              <p class="refresh-error">
                Live refresh unavailable: {activityError}
              </p>
            {/if}
            <p class="section-note">
              <a href={getBasePath() + "/logs"}>View full decision history</a>
            </p>
          </section>
        </TabContent>
      </svelte:fragment>
    </Tabs>
  </ModalBody>
  <ModalFooter
    primaryButtonText={saving ? "Saving…" : "Save labels"}
    primaryButtonDisabled={saving}
    secondaryButtonText="Close"
  />
</ComposedModal>

<ConfirmDialog
  open={Boolean(pendingUnlink)}
  title="Unlink Tailscale peer?"
  label="Confirm identity change"
  confirmText="Unlink peer"
  danger
  busy={tailscaleSaving}
  on:close={() => {
    if (!tailscaleSaving) pendingUnlink = null;
  }}
  on:submit={confirmUnlink}
>
  <p>
    Unlink <strong
      >{pendingUnlink ? peerName(pendingUnlink) : "this peer"}</strong
    >
    from this device? The peer will remain available to link again.
  </p>
</ConfirmDialog>

<style>
  .tab-section {
    display: grid;
    gap: 1rem;
    min-width: 0;
    padding: 1.25rem 0 0.5rem;
  }

  .tab-section h3,
  .tab-section h4,
  .tab-section p {
    margin: 0;
  }

  .section-heading,
  .peer-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .field-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: 1rem;
  }

  .section-note,
  .peer-details,
  .refresh-error {
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
    line-height: 1.45;
  }

  .action-row {
    display: flex;
    align-items: flex-end;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .action-field {
    flex: 1 1 18rem;
    min-width: 0;
  }

  .coverage-summary {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    font-size: 0.875rem;
  }

  .effective-rules {
    font-size: 0.875rem;
  }

  .caveat-list {
    margin: 0;
    padding-left: 1.25rem;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
  }

  .peer-list {
    display: grid;
    gap: 0.75rem;
  }

  .peer-card {
    min-width: 0;
    padding: 1rem;
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .peer-heading > div,
  .peer-details,
  .wol-macs {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .peer-details,
  .wol-macs {
    margin-top: 0.5rem;
  }

  .suggestion {
    margin-top: 0.75rem !important;
    padding: 0.75rem;
    background: var(--cds-layer-01, #f4f4f4);
    color: var(--cds-text-secondary, #525252);
    font-size: 0.8125rem;
  }

  .identity-list {
    max-width: 100%;
    overflow-x: auto;
  }

  .break-value,
  .activity-url {
    overflow-wrap: anywhere;
  }

  .activity-list {
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 0.875rem;
  }

  .activity-list li {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .activity-time,
  .activity-ip {
    color: var(--cds-text-secondary, #525252);
    font-size: 0.8125rem;
    white-space: nowrap;
  }

  :global(.bx--modal-container--lg) {
    max-height: calc(100vh - 3rem);
  }

  :global(.bx--modal-content) {
    min-width: 0;
  }

  :global(.bx--tabs),
  :global(.bx--tabs__nav),
  :global([role="tabpanel"]) {
    max-width: 100%;
    min-width: 0;
  }

  @media (max-width: 42rem) {
    .tab-section {
      padding-top: 1rem;
    }

    .action-row > :global(.bx--btn) {
      width: 100%;
      max-width: none;
    }
  }
</style>
