<script lang="ts">
  import {
    Button,
    ComposedModal,
    InlineLoading,
    InlineNotification,
    ModalBody,
    ModalFooter,
    ModalHeader,
    MultiSelect,
    Select,
    SelectItem,
    Tab,
    TabContent,
    Tabs,
    Tag,
    TextInput,
    Tile,
  } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import Categoryselect from "./categoryselect.svelte";
  import Groupform from "./groupform.svelte";
  import {
    DEFAULT_GROUP_ID,
    copyGroup,
    emptyGroup,
    persistableGroup,
    policySummary,
    ruleSentence,
  } from "./policymodel";
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
  let selectedTab = 0;
  let editingGroupId = "";
  let draft: PolicyGroup = emptyGroup();
  let assigningDevices: Record<string, string[]> = {};
  let deletingGroup: PolicyGroup | null = null;

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
      users = ((await getJSON(USERS_API)).users || [])
        .map((user) => user.username)
        .filter(Boolean);
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
      const [
        templateData,
        groupData,
        categoryData,
        assignmentData,
        deviceData,
      ] = await Promise.all([
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
      gatewaySelection = categories
        .filter((category) => category.enabled)
        .map((category) => category.id);
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  async function reloadAssignments() {
    assignments = (await getJSON(ASSIGNMENTS_API)).assignments || [];
  }

  function openCreate() {
    editingGroupId = "choose";
    draft = emptyGroup();
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
  $: sortedDevices = [...devices].sort((a, b) =>
    deviceLabel(a).localeCompare(deviceLabel(b)),
  );

  // Every device is on exactly one policy: its assignment, or the default.
  function assignmentsByGroup(
    list: DeviceAssignment[],
    known: Device[],
  ): Record<string, string[]> {
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
    const name =
      device.display_name ||
      (device.hostnames || [])[0] ||
      device.ipv4 ||
      device.id;
    if (device.ipv4 && name !== device.ipv4)
      return name + " (" + device.ipv4 + ")";
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
  // different devices cannot overwrite each other. Bulk assignment deliberately
  // reuses that endpoint and then refreshes once, preserving those semantics.
  async function writeDevicePolicy(deviceID: string, groupID: string) {
    const response = await fetch(DEVICES_API + "/" + deviceID + "/assignment", {
      method: "PUT",
      headers: headers(true),
      body: JSON.stringify({
        group_id: groupID === DEFAULT_GROUP_ID ? "" : groupID,
      }),
    });
    if (!response.ok) throw new Error(await responseError(response));
  }

  async function setDevicePolicy(deviceID: string, groupID: string) {
    if (!deviceID) return;
    saving = true;
    error = "";
    success = "";
    try {
      await writeDevicePolicy(deviceID, groupID);
      await reloadAssignments();
      success = deviceName(deviceID) + " now uses " + groupName(groupID) + ".";
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  async function assignSelectedDevices(groupID: string) {
    const selected = assigningDevices[groupID] || [];
    if (!selected.length) return;
    saving = true;
    error = "";
    success = "";
    try {
      for (const deviceID of selected)
        await writeDevicePolicy(deviceID, groupID);
      await reloadAssignments();
      assigningDevices[groupID] = [];
      assigningDevices = { ...assigningDevices };
      success =
        selected.length +
        " device" +
        (selected.length === 1 ? "" : "s") +
        " now use " +
        groupName(groupID) +
        ".";
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
      else
        groups = groups.map((group) => (group.id === saved.id ? saved : group));
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
      gatewaySelection = categories
        .filter((category) => category.enabled)
        .map((category) => category.id);
      success = data.refresh_scheduled
        ? "Categories saved. The gateway is downloading the feeds now, so refresh in a moment to see the new counts."
        : "Categories saved. A download was already running, so the counts update when it finishes.";
    } catch (err) {
      error = err.message;
    } finally {
      savingCategories = false;
    }
  }

  function openDeleteGroup(group: PolicyGroup) {
    deletingGroup = group;
  }

  function closeDeleteGroup() {
    if (!saving) deletingGroup = null;
  }

  async function confirmDeleteGroup() {
    if (!deletingGroup) return;
    const group = deletingGroup;
    saving = true;
    error = "";
    try {
      const response = await fetch(GROUPS_API + "/" + group.id, {
        method: "DELETE",
        headers: headers(),
      });
      if (!response.ok) throw new Error(await responseError(response));
      groups = groups.filter((candidate) => candidate.id !== group.id);
      deletingGroup = null;
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
      const response = await fetch(
        TEMPLATES_API + "/" + template.id + "/apply",
        {
          method: "POST",
          headers: headers(),
        },
      );
      if (!response.ok) throw new Error(await responseError(response));
      const data = await response.json();
      groups = [...groups, copyGroup(data.group)];
      editingGroupId = data.group.id;
      draft = copyGroup(data.group);
      success = template.name + " was added. Review it, then save any changes.";
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
  function verdict(result: PreviewResult): {
    text: string;
    type: "red" | "green" | "gray";
  } {
    const active = result.active;
    if (active.action === "block" || active.url_blocked)
      return { text: "Blocked", type: "red" };
    if (active.action === "allow")
      return { text: "Allowed by policy", type: "green" };
    return { text: "Not blocked by policy", type: "gray" };
  }

  // Starters are the quickest way in, so they lead the page until the first
  // policy of your own exists, then fold away under "Create policy".
  $: customPolicies = groups.filter(
    (group) => group.id !== DEFAULT_GROUP_ID,
  ).length;
  let showStarters = false;
  let expanded: Record<string, boolean> = {};

  onMount(load);
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
    title="Done"
    subtitle={success}
    on:close={() => (success = "")}
  />
{/if}

{#if loading}
  <InlineLoading description="Loading policies..." />
{:else}
  <Tabs bind:selected={selectedTab} aria-label="Policy tools">
    <Tab label="Policies" />
    <Tab label="Test a site" />
    <Tab label="Gateway defaults" />

    <svelte:fragment slot="content">
      <TabContent>
        <section class="section" aria-labelledby="policies-heading">
          <div class="section-heading">
            <div>
              <h3 id="policies-heading">Policies</h3>
              <p class="section-intro">
                Every device follows one policy. Unassigned devices use the
                default policy.
              </p>
            </div>
            <Button size="small" disabled={saving} on:click={openCreate}
              >Create policy</Button
            >
          </div>

          {#if editingGroupId === "choose"}
            <div class="create-panel">
              <div class="create-heading">
                <div>
                  <h4>Create a policy</h4>
                  <p class="section-intro">
                    Start with a tested baseline or build one from scratch.
                  </p>
                </div>
                <Button size="small" kind="ghost" on:click={cancelEdit}
                  >Cancel</Button
                >
              </div>
              {#if sharedLimitations.length}
                <div class="shared-caveats">
                  <InlineNotification
                    kind="info"
                    lowContrast
                    hideCloseButton
                    title="Starter limitations"
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
                          {#each template.categories.slice(0, 3) as category}
                            <Tag size="sm" type="red"
                              >{categoryName(category)}</Tag
                            >
                          {/each}
                          {#if template.categories.length > 3}
                            <Tag size="sm"
                              >+{template.categories.length - 3} more</Tag
                            >
                          {/if}
                        </div>
                      {/if}
                      {#if template.limitations?.length}
                        <p class="tile-caveat">
                          {template.limitations.join(" ")}
                        </p>
                      {/if}
                      <Button
                        size="small"
                        kind={templateApplied(template) ? "ghost" : "secondary"}
                        disabled={saving ||
                          !template.available ||
                          templateApplied(template)}
                        on:click={() => applyTemplate(template)}
                      >
                        {templateApplied(template)
                          ? "Already added"
                          : "Use starter"}
                      </Button>
                    </div>
                  </Tile>
                {/each}
                <Tile>
                  <div class="template-tile blank-template">
                    <h4>Blank policy</h4>
                    <p class="tile-description">
                      Choose every category, domain, schedule, and advanced rule
                      yourself.
                    </p>
                    <Button size="small" kind="tertiary" on:click={beginCreate}
                      >Start blank</Button
                    >
                  </div>
                </Tile>
              </div>
            </div>
          {/if}

          {#if editingGroupId === "new"}
            <div class="group-slot editor-slot">
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

          <div class="policy-list">
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
                            {(assignedByGroup[group.id] || []).length} device{(
                              assignedByGroup[group.id] || []
                            ).length === 1
                              ? ""
                              : "s"}{group.users.length
                              ? " · " +
                                group.users.length +
                                " proxy user" +
                                (group.users.length === 1 ? "" : "s")
                              : ""}
                          </span>
                        </div>
                        <p class="summary-line">
                          {policySummary(group, categoryName)}
                        </p>
                        {#if group.id === DEFAULT_GROUP_ID}
                          <p class="muted">
                            New and unassigned devices use this policy
                            automatically.
                          </p>
                        {:else if !(assignedByGroup[group.id] || []).length && !group.users.length}
                          <p class="muted">No devices use this policy yet.</p>
                        {/if}

                        <button
                          type="button"
                          class="link-button"
                          on:click={() =>
                            (expanded[group.id] = !expanded[group.id])}
                        >
                          {expanded[group.id] ? "Hide details" : "View details"}
                        </button>

                        {#if expanded[group.id]}
                          <div class="policy-details">
                            {#if group.description}<p>
                                {group.description}
                              </p>{/if}
                            {#if group.blocked_categories.length || group.blocked_domains.length}
                              <div class="tags">
                                <span class="tags-label">Blocks</span>
                                {#each group.blocked_categories as category}<Tag
                                    size="sm"
                                    type="red">{categoryName(category)}</Tag
                                  >{/each}
                                {#each group.blocked_domains as domain}<Tag
                                    size="sm"
                                    type="outline">{domain}</Tag
                                  >{/each}
                              </div>
                            {/if}
                            {#if group.allowed_domains.length}
                              <div class="tags">
                                <span class="tags-label">Always allows</span>
                                {#each group.allowed_domains as domain}<Tag
                                    size="sm"
                                    type="green">{domain}</Tag
                                  >{/each}
                              </div>
                            {/if}
                            {#if group.rules.length}
                              <ol class="rule-list">
                                {#each group.rules as rule (rule.id)}
                                  <li class:off={!rule.enabled}>
                                    {rule.name
                                      ? rule.name + ": "
                                      : ""}{ruleSentence(
                                      rule,
                                      categoryName,
                                    )}{rule.enabled ? "" : " (off)"}
                                  </li>
                                {/each}
                              </ol>
                            {/if}
                            {#if group.users.length}<p class="muted">
                                Proxy users: {group.users.join(", ")}
                              </p>{/if}
                          </div>
                        {/if}

                        <div class="assignment-panel">
                          <div class="assignment-heading">
                            <h5>Assigned devices</h5>
                            <span class="device-count"
                              >{(assignedByGroup[group.id] || []).length}</span
                            >
                          </div>
                          {#if (assignedByGroup[group.id] || []).length}
                            <div class="tags assigned-tags">
                              {#each assignedByGroup[group.id] as deviceID (deviceID)}
                                {#if group.id === DEFAULT_GROUP_ID}
                                  <Tag size="sm">{deviceName(deviceID)}</Tag>
                                {:else}
                                  <Tag
                                    size="sm"
                                    filter
                                    title="Move to the default policy"
                                    on:close={() =>
                                      setDevicePolicy(
                                        deviceID,
                                        DEFAULT_GROUP_ID,
                                      )}
                                  >
                                    {deviceName(deviceID)}
                                  </Tag>
                                {/if}
                              {/each}
                            </div>
                          {:else}
                            <p class="muted compact-note">
                              No assigned devices.
                            </p>
                          {/if}

                          {#if group.id !== DEFAULT_GROUP_ID && devicesNotOn(group.id).length}
                            <div class="assign-row">
                              <MultiSelect
                                size="sm"
                                titleText="Add devices"
                                label="Select devices"
                                items={devicesNotOn(group.id).map((device) => ({
                                  id: device.id,
                                  text: deviceLabel(device),
                                }))}
                                selectedIds={assigningDevices[group.id] || []}
                                on:select={(event) => {
                                  assigningDevices[group.id] =
                                    event.detail.selectedIds.map(String);
                                  assigningDevices = { ...assigningDevices };
                                }}
                              />
                              <Button
                                size="small"
                                kind="tertiary"
                                disabled={saving ||
                                  !(assigningDevices[group.id] || []).length}
                                on:click={() => assignSelectedDevices(group.id)}
                                >Assign selected</Button
                              >
                            </div>
                          {/if}
                        </div>
                      </div>
                      <div class="group-actions">
                        <Button
                          size="small"
                          kind="ghost"
                          disabled={saving}
                          on:click={() => beginEdit(group)}>Edit</Button
                        >
                        {#if group.id !== DEFAULT_GROUP_ID}
                          <Button
                            size="small"
                            kind="danger-ghost"
                            disabled={saving}
                            on:click={() => openDeleteGroup(group)}
                            >Delete</Button
                          >
                        {/if}
                      </div>
                    </div>
                  {/if}
                </Tile>
              </div>
            {/each}
          </div>
        </section>
      </TabContent>

      <TabContent>
        <section class="section tab-section" aria-labelledby="test-heading">
          <h3 id="test-heading">Test a site</h3>
          <p class="section-intro">
            Check whether a site is blocked for a device right now and which
            setting decides it. Nothing is changed.
          </p>
          <Tile>
            <div class="test-row">
              <Select size="sm" labelText="Device" bind:selected={testDevice}>
                <SelectItem
                  value=""
                  text="An unknown device (default policy)"
                />
                {#each sortedDevices as device (device.id)}<SelectItem
                    value={device.id}
                    text={deviceLabel(device)}
                  />{/each}
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
              <Button
                size="small"
                disabled={testing || !testTarget.trim()}
                on:click={runTest}>Test</Button
              >
            </div>
            {#if testing}<InlineLoading description="Evaluating..." />{/if}
            {#if testError}<p class="error-text">{testError}</p>{/if}
            {#if testResult}
              <div class="test-result">
                <div class="group-title">
                  <Tag type={verdict(testResult).type}
                    >{verdict(testResult).text}</Tag
                  ><span
                    >by policy <strong>{testResult.active.group_name}</strong
                    ></span
                  >
                </div>
                {#if testResult.active.reason}<p>
                    Decided by {testResult.active.reason}.
                  </p>{/if}
                {#if testResult.active.url_blocked}<p>
                    The URL matches a blocked pattern of this policy.
                  </p>{/if}
                {#if testResult.active.proxy_only.length}
                  <p class="muted">
                    A rule for this site checks {testResult.active.proxy_only
                      .join(", ")
                      .replace("url_regex", "the URL")
                      .replace("content_type", "the response type")
                      .replace("user", "the proxy login")}, which only the proxy
                    can see. Test the Proxy path with a full URL.
                  </p>
                {/if}
                {#if testResult.active.safe_search}<p class="muted">
                    Safe search is on for this policy.
                  </p>{/if}
                <ol class="trace">
                  {#each testResult.active.trace as step}<li
                      class:applied={step.applied}
                    >
                      {step.stage.replace(/_/g, " ")}: {step.detail ||
                        step.action}
                    </li>{/each}
                </ol>
                <p class="muted">{testResult.identity.explanation}</p>
              </div>
            {/if}
          </Tile>
        </section>
      </TabContent>

      <TabContent>
        <section class="section tab-section" aria-labelledby="defaults-heading">
          <div class="section-heading">
            <div>
              <h3 id="defaults-heading">Gateway defaults</h3>
              <p class="section-intro">
                These categories are blocked for every device on every policy. A
                policy's always-allowed domains can still exempt a site.
              </p>
            </div>
            <Button
              size="small"
              disabled={savingCategories}
              on:click={saveGatewayCategories}>Save categories</Button
            >
          </div>
          <Tile>
            <Categoryselect
              {categories}
              selection={gatewaySelection}
              disabled={savingCategories}
              on:change={(event) => (gatewaySelection = event.detail)}
            />
          </Tile>
        </section>
      </TabContent>
    </svelte:fragment>
  </Tabs>
{/if}

<ComposedModal
  open={!!deletingGroup}
  danger
  size="sm"
  preventCloseOnClickOutside
  on:close={closeDeleteGroup}
  on:submit={confirmDeleteGroup}
>
  <ModalHeader title="Delete policy?" label="This action cannot be undone" />
  <ModalBody>
    <p>
      Delete <strong>{deletingGroup?.name}</strong>? Devices assigned to it will
      move to the default policy.
    </p>
  </ModalBody>
  <ModalFooter
    danger
    primaryButtonText={saving ? "Deleting..." : "Delete policy"}
    primaryButtonDisabled={saving}
    secondaryButtonText="Cancel"
  />
</ComposedModal>

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
  :global(.bx--tab-content) {
    padding: 1.5rem 0 0;
  }
  .section {
    margin-bottom: 2.5rem;
  }
  .tab-section {
    max-width: 72rem;
  }
  .create-panel {
    margin-bottom: 1.5rem;
    padding: 1rem;
    border-left: 3px solid #0f62fe;
    background: #f4f4f4;
  }
  .create-heading,
  .assignment-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }
  .policy-list {
    display: grid;
    gap: 0.75rem;
  }
  .editor-slot {
    scroll-margin-top: 1rem;
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
  .policy-details {
    margin-bottom: 1rem;
    padding: 0.75rem 1rem;
    background: #f4f4f4;
  }
  .assignment-panel {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid #e0e0e0;
  }
  .assignment-heading {
    justify-content: flex-start;
    align-items: baseline;
    margin-bottom: 0.25rem;
  }
  .assigned-tags {
    max-height: 5.25rem;
    overflow-y: auto;
  }
  .compact-note {
    margin-bottom: 0.5rem;
  }
  .assign-row {
    display: grid;
    grid-template-columns: minmax(16rem, 28rem) auto;
    align-items: end;
    gap: 0.5rem;
    max-width: 42rem;
    margin-top: 0.75rem;
  }
  .assign-row :global(.bx--form-item) {
    min-width: 0;
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
  @media (max-width: 48rem) {
    .test-row,
    .assign-row {
      grid-template-columns: 1fr;
    }
    .group-row {
      flex-direction: column;
    }
    .group-actions {
      align-self: flex-end;
    }
  }
  @media (max-width: 30rem) {
    :global(.bx--tab-content) {
      padding-top: 1rem;
    }
    .section-heading :global(.bx--btn),
    .create-heading :global(.bx--btn),
    .assign-row :global(.bx--btn) {
      width: 100%;
      max-width: none;
    }
    .create-heading,
    .section-heading {
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
