<script lang="ts">
  import {
    Button,
    InlineLoading,
    InlineNotification,
    Select,
    SelectItem,
    Tag,
    TextInput,
    Tile,
  } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import Categoryselect from "./categoryselect.svelte";
  import Groupform from "./groupform.svelte";
  import { DEFAULT_GROUP_ID, copyGroup, emptyGroup, persistableGroup, policySummary, ruleSentence } from "./policymodel";
  import type {
    CategoryStatus,
    Device,
    DeviceAssignment,
    PolicyGroup,
    PolicyTemplate,
    PreviewResult,
    SchedulePreset,
  } from "./policymodel";

  const TEMPLATES_API = getBasePath() + "/api/policy/templates";
  const GROUPS_API = getBasePath() + "/api/policy/groups";
  const CATEGORIES_API = getBasePath() + "/api/policy/categories";
  const ASSIGNMENTS_API = getBasePath() + "/api/policy/assignments";
  const PREVIEW_API = getBasePath() + "/api/policy/preview";
  const DEVICES_API = getBasePath() + "/api/devices";
  const PRESETS_API = getBasePath() + "/api/policy/schedule-presets";
  const USERS_API = getBasePath() + "/api/users";
  const TIMEZONE_API = getBasePath() + "/api/settings/timezone";

  let templates: PolicyTemplate[] = [];
  // Caveats the API states for the whole catalog, shown once above the grid.
  let sharedLimitations: string[] = [];
  let groups: PolicyGroup[] = [];
  let categories: CategoryStatus[] = [];
  let assignments: DeviceAssignment[] = [];
  let devices: Device[] = [];
  let gatewaySelection: string[] = [];
  let schedulePresets: SchedulePreset[] = [];
  // Authenticated proxy users, so a policy can name the logins the gateway can
  // actually identify instead of a name typed from memory.
  let users: string[] = [];
  let timezone = "UTC";
  let loading = true;
  let saving = false;
  let savingCategories = false;
  let error = "";
  let success = "";
  let editingGroupId = "";
  let draft: PolicyGroup = emptyGroup();
  let assigningDevice: Record<string, string> = {};

  // The site tester runs the same evaluator that enforces traffic, so what it
  // shows is what a device gets.
  let testDevice = "";
  let testTarget = "";
  let testLayer = "dns";
  let testing = false;
  let testResult: PreviewResult | null = null;
  let testError = "";

  function token(): string {
    return localStorage.getItem("jwt") || "";
  }

  function headers(json = false): Record<string, string> {
    const result: Record<string, string> = {
      Authorization: "Bearer " + token(),
    };
    if (json) result["Content-Type"] = "application/json";
    return result;
  }

  async function responseError(response: Response): Promise<string> {
    const body = await response.text();
    try {
      const parsed = JSON.parse(body);
      if (parsed && parsed.error) return parsed.error;
    } catch {
      // Use the response body below when it is not JSON.
    }
    return body || "Request failed (" + response.status + ")";
  }

  async function getJSON(url: string) {
    const response = await fetch(url, { headers: headers() });
    if (!response.ok) throw new Error(await responseError(response));
    return response.json();
  }

  // The schedule presets, the user logins, and the installation time zone
  // support the rule editor. None of them is worth failing the page over, so
  // each is read on its own and the control that needs it is simply absent.
  async function loadSupporting() {
    try {
      schedulePresets = (await getJSON(PRESETS_API)).presets || [];
    } catch {
      schedulePresets = [];
    }
    try {
      users = ((await getJSON(USERS_API)).users || []).map((user) => user.username).filter(Boolean);
    } catch {
      users = [];
    }
    try {
      const data = await getJSON(TIMEZONE_API);
      if (data.Value) timezone = data.Value;
    } catch {
      // Schedules fall back to UTC, which the evaluator also uses.
    }
  }

  async function load() {
    loading = true;
    error = "";
    try {
      const [templateData, groupData, categoryData, assignmentData, deviceData] = await Promise.all([
        getJSON(TEMPLATES_API),
        getJSON(GROUPS_API),
        getJSON(CATEGORIES_API),
        getJSON(ASSIGNMENTS_API),
        getJSON(DEVICES_API),
        loadSupporting(),
      ]);
      templates = templateData.templates || [];
      sharedLimitations = templateData.shared_limitations || [];
      groups = (groupData.groups || []).map(copyGroup);
      categories = categoryData.categories || [];
      assignments = assignmentData.assignments || [];
      devices = deviceData.devices || [];
      gatewaySelection = categories.filter((category) => category.enabled).map((category) => category.id);
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  async function reloadAssignments() {
    assignments = (await getJSON(ASSIGNMENTS_API)).assignments || [];
  }

  function beginCreate() {
    editingGroupId = "new";
    draft = emptyGroup();
  }

  function beginEdit(group: PolicyGroup) {
    editingGroupId = group.id;
    draft = copyGroup(group);
  }

  function cancelEdit() {
    editingGroupId = "";
    draft = emptyGroup();
  }

  function categoryName(id: string): string {
    const found = categories.find((category) => category.id === id);
    return found ? found.name : id;
  }

  // Svelte re-renders a template expression only when a variable that the
  // expression names changes, so the assignments are grouped into a plain map
  // the template reads directly.
  $: assignedByGroup = assignmentsByGroup(assignments, devices);
  $: sortedDevices = [...devices].sort((a, b) => deviceLabel(a).localeCompare(deviceLabel(b)));

  // Every device is on exactly one policy: its assignment, or the default.
  function assignmentsByGroup(list: DeviceAssignment[], known: Device[]): Record<string, string[]> {
    const map: Record<string, string[]> = { [DEFAULT_GROUP_ID]: [] };
    const assigned = new Set<string>();
    for (const assignment of list) {
      (map[assignment.group_id] ||= []).push(assignment.device_id);
      assigned.add(assignment.device_id);
    }
    for (const device of known) {
      if (!assigned.has(device.id)) map[DEFAULT_GROUP_ID].push(device.id);
    }
    return map;
  }

  // A device is named the way discovery recorded it: the administrator's label
  // first, then a hostname, then the address it was last seen on.
  function deviceLabel(device: Device): string {
    const name = device.display_name || (device.hostnames || [])[0] || device.ipv4 || device.id;
    if (device.ipv4 && name !== device.ipv4) return name + " (" + device.ipv4 + ")";
    return name;
  }

  function deviceName(deviceID: string): string {
    const device = devices.find((candidate) => candidate.id === deviceID);
    return device ? deviceLabel(device) : deviceID;
  }

  function devicesNotOn(groupID: string): Device[] {
    const on = new Set(assignedByGroup[groupID] || []);
    return sortedDevices.filter((device) => !on.has(device.id));
  }

  // Assignment writes one device at a time, so two administrators editing
  // different devices cannot overwrite each other.
  async function setDevicePolicy(deviceID: string, groupID: string) {
    if (!deviceID) return;
    saving = true;
    error = "";
    success = "";
    try {
      const response = await fetch(DEVICES_API + "/" + deviceID + "/assignment", {
        method: "PUT",
        headers: headers(true),
        body: JSON.stringify({ group_id: groupID === DEFAULT_GROUP_ID ? "" : groupID }),
      });
      if (!response.ok) throw new Error(await responseError(response));
      await reloadAssignments();
      success = deviceName(deviceID) + " now uses " + groupName(groupID) + ".";
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  function groupName(groupID: string): string {
    return groups.find((group) => group.id === groupID)?.name || groupID;
  }

  async function saveGroup() {
    if (!draft.name.trim()) {
      error = "A policy needs a name.";
      return;
    }
    saving = true;
    error = "";
    success = "";
    try {
      const isNew = editingGroupId === "new";
      const url = isNew ? GROUPS_API : GROUPS_API + "/" + editingGroupId;
      const response = await fetch(url, {
        method: isNew ? "POST" : "PUT",
        headers: headers(true),
        body: JSON.stringify(persistableGroup(draft)),
      });
      if (!response.ok) throw new Error(await responseError(response));
      const saved = copyGroup((await response.json()).group);
      if (isNew) groups = [...groups, saved];
      else groups = groups.map((group) => (group.id === saved.id ? saved : group));
      success = "Policy saved. It applies to new DNS lookups within a minute.";
      cancelEdit();
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function saveGatewayCategories() {
    savingCategories = true;
    error = "";
    success = "";
    try {
      const response = await fetch(CATEGORIES_API, {
        method: "PUT",
        headers: headers(true),
        body: JSON.stringify({ categories: gatewaySelection }),
      });
      if (!response.ok) throw new Error(await responseError(response));
      const data = await response.json();
      categories = data.categories || [];
      gatewaySelection = categories.filter((category) => category.enabled).map((category) => category.id);
      success = data.refresh_scheduled
        ? "Categories saved. The gateway is downloading the feeds now, so refresh in a moment to see the new counts."
        : "Categories saved. A download was already running, so the counts update when it finishes.";
    } catch (err) {
      error = err.message;
    } finally {
      savingCategories = false;
    }
  }

  async function deleteGroup(group: PolicyGroup) {
    if (!confirm('Delete the policy "' + group.name + '"? Its devices move to the default policy.')) return;
    saving = true;
    error = "";
    try {
      const response = await fetch(GROUPS_API + "/" + group.id, {
        method: "DELETE",
        headers: headers(),
      });
      if (!response.ok) throw new Error(await responseError(response));
      groups = groups.filter((candidate) => candidate.id !== group.id);
      await reloadAssignments();
      success = "Policy deleted; its devices now use the default policy.";
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function applyTemplate(template: PolicyTemplate) {
    saving = true;
    error = "";
    success = "";
    try {
      const response = await fetch(TEMPLATES_API + "/" + template.id + "/apply", {
        method: "POST",
        headers: headers(),
      });
      if (!response.ok) throw new Error(await responseError(response));
      const data = await response.json();
      groups = [...groups, copyGroup(data.group)];
      success = template.name + ' was added as a policy. Assign devices to it under "Devices on this policy".';
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  function templateApplied(template: PolicyTemplate): boolean {
    return groups.some((group) => group.id === template.group_id);
  }

  async function runTest() {
    const target = testTarget.trim();
    if (!target) return;
    testing = true;
    testError = "";
    testResult = null;
    const device = devices.find((candidate) => candidate.id === testDevice);
    const isURL = target.includes("/");
    const url = isURL && !target.includes("://") ? "https://" + target : target;
    const body: Record<string, string> = {
      client_ip: device?.ipv4 || "",
      protocol: testLayer,
    };
    if (isURL) body.url = url;
    else body.domain = target;
    try {
      const response = await fetch(PREVIEW_API, {
        method: "POST",
        headers: headers(true),
        body: JSON.stringify(body),
      });
      if (!response.ok) throw new Error(await responseError(response));
      testResult = await response.json();
    } catch (err) {
      testError = err.message;
    } finally {
      testing = false;
    }
  }

  // The verdict an administrator reads first. A URL condition on the proxy
  // is a block for the tested URL only.
  function verdict(result: PreviewResult): { text: string; type: "red" | "green" | "gray" } {
    const active = result.active;
    if (active.action === "block" || active.url_blocked) return { text: "Blocked", type: "red" };
    if (active.action === "allow") return { text: "Allowed by policy", type: "green" };
    return { text: "Not blocked by policy", type: "gray" };
  }

  // Starters are the quickest way in, so they lead the page until the first
  // policy of your own exists, then fold away under "Create policy".
  $: customPolicies = groups.filter((group) => group.id !== DEFAULT_GROUP_ID).length;
  let showStarters = false;
  let expanded: Record<string, boolean> = {};

  onMount(load);
</script>

{#if error}
  <InlineNotification kind="error" title="Error" subtitle={error} on:close={() => (error = "")} />
{/if}
{#if success}
  <InlineNotification kind="success" title="Done" subtitle={success} on:close={() => (success = "")} />
{/if}

{#if loading}
  <InlineLoading description="Loading policies..." />
{:else}
  <section class="section">
    <div class="section-heading">
      <div>
        <h3>Policies</h3>
      </div>
      <div class="heading-actions">
        {#if customPolicies}
          <Button size="small" kind="ghost" on:click={() => (showStarters = !showStarters)}
            >{showStarters ? "Hide starters" : "Use a starter"}</Button
          >
        {/if}
        <Button size="small" kind="secondary" disabled={saving} on:click={beginCreate}>Create policy</Button>
      </div>
    </div>

    {#if !customPolicies || showStarters}
      <div class="starters">
        <h4>Start from a starter</h4>
        <p class="section-intro">Adds an ordinary policy you can edit. Adding it again never overwrites your changes.</p>
        {#if sharedLimitations.length}
          <div class="shared-caveats">
            <InlineNotification
              kind="info"
              lowContrast
              hideCloseButton
              title="Applies to every starter"
              subtitle={sharedLimitations.join(" ")}
            />
          </div>
        {/if}
        <div class="template-grid">
          {#each templates as template (template.id)}
            <Tile>
              <div class="template-tile">
                <h4>{template.name}</h4>
                <p class="tile-description">{template.description}</p>
                {#if template.categories?.length}
                  <div class="tags">
                    {#each template.categories as category}
                      <Tag size="sm" type="red">{categoryName(category)}</Tag>
                    {/each}
                  </div>
                {/if}
                {#if template.limitations?.length}
                  <p class="tile-caveat">{template.limitations.join(" ")}</p>
                {/if}
                <Button
                  size="small"
                  kind={templateApplied(template) ? "ghost" : "primary"}
                  disabled={saving || !template.available || templateApplied(template)}
                  on:click={() => applyTemplate(template)}
                >
                  {templateApplied(template) ? "Added" : "Add policy"}
                </Button>
              </div>
            </Tile>
          {/each}
        </div>
      </div>
    {/if}

    {#if editingGroupId === "new"}
      <div class="group-slot">
        <Tile>
          <h4>New policy</h4>
          <Groupform
            {draft}
            {categories}
            {users}
            {schedulePresets}
            {timezone}
            {saving}
            onSave={saveGroup}
            onCancel={cancelEdit}
          />
        </Tile>
      </div>
    {/if}

    {#each groups as group (group.id)}
      <div class="group-slot">
        <Tile>
          {#if editingGroupId === group.id}
            <h4>Edit {group.name}</h4>
            <Groupform
              {draft}
              {categories}
              {users}
              {schedulePresets}
              {timezone}
              {saving}
              onSave={saveGroup}
              onCancel={cancelEdit}
            />
          {:else}
            <div class="group-row">
              <div class="group-summary">
                <div class="group-title">
                  <h4>{group.name}</h4>
                  {#if group.id === DEFAULT_GROUP_ID}
                    <Tag size="sm" type="blue">default</Tag>
                  {/if}
                  <span class="device-count">
                    {(assignedByGroup[group.id] || []).length} device{(assignedByGroup[group.id] || []).length === 1 ? "" : "s"}{group.users.length ? " · " + group.users.length + " proxy user" + (group.users.length === 1 ? "" : "s") : ""}
                  </span>
                </div>
                <p class="summary-line">{policySummary(group, categoryName)}</p>
                {#if group.id === DEFAULT_GROUP_ID}
                  <p class="muted">Every device uses exactly one policy. Devices you haven't assigned use this one.</p>
                {:else if !(assignedByGroup[group.id] || []).length && !group.users.length}
                  <p class="muted">No devices yet, so this policy changes nothing until you add one.</p>
                {/if}

                <button type="button" class="link-button" on:click={() => (expanded[group.id] = !expanded[group.id])}>
                  {expanded[group.id] ? "Hide details" : "Details and devices"}
                </button>

                {#if expanded[group.id]}
                  {#if group.description}
                    <p>{group.description}</p>
                  {/if}
                  {#if group.blocked_categories.length || group.blocked_domains.length}
                    <div class="tags">
                      <span class="tags-label">Blocks</span>
                      {#each group.blocked_categories as category}
                        <Tag size="sm" type="red">{categoryName(category)}</Tag>
                      {/each}
                      {#each group.blocked_domains as domain}
                        <Tag size="sm" type="outline">{domain}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if group.allowed_domains.length}
                    <div class="tags">
                      <span class="tags-label">Always allows</span>
                      {#each group.allowed_domains as domain}
                        <Tag size="sm" type="green">{domain}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if group.rules.length}
                    <ol class="rule-list">
                      {#each group.rules as rule (rule.id)}
                        <li class:off={!rule.enabled}>
                          {rule.name ? rule.name + ": " : ""}{ruleSentence(rule, categoryName)}{rule.enabled ? "" : " (off)"}
                        </li>
                      {/each}
                    </ol>
                  {/if}

                  <div class="group-assignment">
                    <h5>Devices on this policy ({(assignedByGroup[group.id] || []).length})</h5>
                    {#if (assignedByGroup[group.id] || []).length}
                      <div class="tags">
                        {#each assignedByGroup[group.id] as deviceID (deviceID)}
                          {#if group.id === DEFAULT_GROUP_ID}
                            <Tag size="sm">{deviceName(deviceID)}</Tag>
                          {:else}
                            <Tag
                              size="sm"
                              filter
                              title="Move to the default policy"
                              on:close={() => setDevicePolicy(deviceID, DEFAULT_GROUP_ID)}>{deviceName(deviceID)}</Tag
                            >
                          {/if}
                        {/each}
                      </div>
                    {/if}
                    {#if group.users.length}
                      <p class="muted">Proxy users: {group.users.join(", ")}</p>
                    {/if}
                  </div>
                {/if}

                {#if group.id !== DEFAULT_GROUP_ID && devicesNotOn(group.id).length}
                  <div class="assign-row">
                    <Select size="sm" hideLabel labelText="Add a device" bind:selected={assigningDevice[group.id]}>
                      <SelectItem value="" text="Add a device..." />
                      {#each devicesNotOn(group.id) as device (device.id)}
                        <SelectItem value={device.id} text={deviceLabel(device)} />
                      {/each}
                    </Select>
                    <Button
                      size="small"
                      kind="tertiary"
                      disabled={saving || !assigningDevice[group.id]}
                      on:click={() => {
                        setDevicePolicy(assigningDevice[group.id], group.id);
                        assigningDevice[group.id] = "";
                      }}>Add</Button
                    >
                  </div>
                {/if}
              </div>
              <div class="group-actions">
                <Button size="small" kind="ghost" disabled={saving} on:click={() => beginEdit(group)}>Edit</Button>
                {#if group.id !== DEFAULT_GROUP_ID}
                  <Button size="small" kind="danger-tertiary" disabled={saving} on:click={() => deleteGroup(group)}
                    >Delete</Button
                  >
                {/if}
              </div>
            </div>
          {/if}
        </Tile>
      </div>
    {/each}
  </section>

  <details class="section fold">
    <summary><h3>Test a site</h3></summary>
    <p class="section-intro">
      Check whether a site is blocked for a device right now, and which setting decides it. Nothing is changed.
    </p>
    <Tile>
      <div class="test-row">
        <Select size="sm" labelText="Device" bind:selected={testDevice}>
          <SelectItem value="" text="An unknown device (default policy)" />
          {#each sortedDevices as device (device.id)}
            <SelectItem value={device.id} text={deviceLabel(device)} />
          {/each}
        </Select>
        <TextInput
          size="sm"
          labelText="Domain or URL"
          placeholder="youtube.com/shorts/abc"
          bind:value={testTarget}
          on:keydown={(event) => event.key === "Enter" && runTest()}
        />
        <Select size="sm" labelText="Path" bind:selected={testLayer}>
          <SelectItem value="dns" text="DNS" />
          <SelectItem value="proxy" text="Proxy" />
        </Select>
        <Button size="small" disabled={testing || !testTarget.trim()} on:click={runTest}>Test</Button>
      </div>
      {#if testing}
        <InlineLoading description="Evaluating..." />
      {/if}
      {#if testError}
        <p class="error-text">{testError}</p>
      {/if}
      {#if testResult}
        <div class="test-result">
          <div class="group-title">
            <Tag type={verdict(testResult).type}>{verdict(testResult).text}</Tag>
            <span>by policy <strong>{testResult.active.group_name}</strong></span>
          </div>
          {#if testResult.active.reason}
            <p>Decided by {testResult.active.reason}.</p>
          {/if}
          {#if testResult.active.url_blocked}
            <p>The URL matches a blocked pattern of this policy.</p>
          {/if}
          {#if testResult.active.proxy_only.length}
            <p class="muted">
              A rule for this site checks {testResult.active.proxy_only.join(", ").replace("url_regex", "the URL").replace(
                "content_type",
                "the response type",
              ).replace("user", "the proxy login")}, which only the proxy can see. Test the Proxy path with a full URL.
            </p>
          {/if}
          {#if testResult.active.safe_search}
            <p class="muted">Safe search is on for this policy.</p>
          {/if}
          <ol class="trace">
            {#each testResult.active.trace as step}
              <li class:applied={step.applied}>{step.stage.replace(/_/g, " ")}: {step.detail || step.action}</li>
            {/each}
          </ol>
          <p class="muted">{testResult.identity.explanation}</p>
        </div>
      {/if}
    </Tile>
  </details>

  <details class="section fold">
    <summary><h3>Gateway-wide categories</h3></summary>
    <div class="section-heading">
      <div>
        <p class="section-intro">
          Blocked for every device on every policy, like the global blocklist. A policy's always-allowed domains can
          still exempt a site.
        </p>
      </div>
      <Button size="small" disabled={savingCategories} on:click={saveGatewayCategories}>Save categories</Button>
    </div>
    <Categoryselect
      {categories}
      selection={gatewaySelection}
      disabled={savingCategories}
      on:change={(event) => (gatewaySelection = event.detail)}
    />
  </details>
{/if}

<style>
  /* This build ships Carbon v10's compiled g10 theme, whose colors are literal
     values rather than --cds-* custom properties: #161616 text-01, #525252
     text-02, #da1e28 support-01. */
  h3,
  h4,
  h5,
  p {
    margin-top: 0;
  }
  h3 {
    margin-bottom: 0.25rem;
    font-size: 1.25rem;
    font-weight: 400;
    line-height: 1.75rem;
  }
  h4 {
    margin-bottom: 0.25rem;
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.375rem;
  }
  h5 {
    margin: 0 0 0.25rem;
    color: #525252;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.32px;
    line-height: 1rem;
  }
  .section {
    margin-bottom: 2.5rem;
  }
  .fold > summary {
    margin-bottom: 1rem;
    cursor: pointer;
  }
  .fold > summary h3 {
    display: inline;
  }
  .heading-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
  }
  .starters {
    margin-bottom: 1.5rem;
  }
  .device-count {
    color: #525252;
    font-size: 0.75rem;
  }
  .summary-line {
    margin-bottom: 0.25rem;
    color: #161616 !important;
  }
  .link-button {
    padding: 0;
    border: 0;
    background: none;
    color: #0f62fe;
    cursor: pointer;
    font-size: 0.875rem;
    margin: 0.25rem 0 0.5rem;
  }
  .section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 1rem;
    margin: 0 0 1rem;
  }
  .section-intro,
  .tile-description,
  .group-summary p,
  .test-result p,
  .muted {
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  .section-intro {
    max-width: 48rem;
    margin-bottom: 1rem;
  }
  .tile-caveat {
    margin: 0 0 0.75rem;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  .shared-caveats {
    margin-bottom: 1rem;
  }
  .template-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: 1rem;
  }
  .template-grid :global(.bx--tile) {
    display: flex;
    height: 100%;
  }
  .template-tile {
    display: flex;
    flex: 1;
    flex-direction: column;
  }
  .template-tile :global(.bx--btn) {
    align-self: flex-start;
    margin-top: auto;
  }
  .group-slot {
    margin-bottom: 0.75rem;
  }
  .group-row {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
  }
  .group-summary {
    flex: 1;
    min-width: 0;
  }
  .group-title {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }
  .group-title h4 {
    margin: 0;
  }
  .group-title :global(.bx--tag),
  .tags :global(.bx--tag) {
    margin: 0;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  .tags-label {
    margin-right: 0.25rem;
    color: #525252;
    font-size: 0.75rem;
  }
  .rule-list {
    margin: 0.5rem 0 0 1.25rem;
    padding: 0;
    font-size: 0.875rem;
    line-height: 1.5rem;
    list-style: decimal;
  }
  .rule-list .off {
    color: #8d8d8d;
  }
  .group-assignment {
    margin-top: 1rem;
  }
  .assign-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    max-width: 28rem;
  }
  .assign-row :global(.bx--form-item) {
    flex: 1;
  }
  .group-actions {
    display: flex;
    align-items: flex-start;
    flex-shrink: 0;
    gap: 0.5rem;
  }
  .test-row {
    display: grid;
    grid-template-columns: minmax(12rem, 1fr) minmax(12rem, 2fr) 7rem auto;
    align-items: end;
    gap: 1rem;
  }
  @media (max-width: 42rem) {
    .test-row {
      grid-template-columns: 1fr;
    }
    .group-row {
      flex-direction: column;
    }
  }
  .test-result {
    margin-top: 1rem;
  }
  .trace {
    margin: 0.5rem 0 0.5rem 1.25rem;
    padding: 0;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.25rem;
    list-style: decimal;
  }
  .trace .applied {
    color: #161616;
    font-weight: 600;
  }
  .error-text {
    color: #da1e28;
    font-size: 0.875rem;
  }
</style>
