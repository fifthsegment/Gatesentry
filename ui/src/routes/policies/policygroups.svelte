<script lang="ts">
  import {
    Button,
    Column,
    InlineLoading,
    InlineNotification,
    Row,
    Tag,
    Tile,
  } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import Categoryselect from "./categoryselect.svelte";
  import Groupform from "./groupform.svelte";
  import { conditionLabel, copyGroup, emptyGroup, persistableGroup } from "./policymodel";
  import type {
    CategoryStatus,
    Device,
    DeviceAssignment,
    PolicyGroup,
    PolicyTemplate,
    SchedulePreset,
  } from "./policymodel";

  const TEMPLATES_API = getBasePath() + "/api/policy/templates";
  const GROUPS_API = getBasePath() + "/api/policy/groups";
  const CATEGORIES_API = getBasePath() + "/api/policy/categories";
  const ASSIGNMENTS_API = getBasePath() + "/api/policy/assignments";
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
  // Authenticated proxy users, so a rule can be scoped to the logins the
  // gateway can actually identify instead of to a name typed from memory.
  let users: string[] = [];
  let timezone = "UTC";
  let loading = true;
  let saving = false;
  let savingCategories = false;
  let error = "";
  let success = "";
  let editingGroupId = "";
  let draft: PolicyGroup = emptyGroup();

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

  // The schedule presets, the user logins, and the installation time zone
  // support the rule editor. None of them is worth failing the policy list
  // over, so each is read on its own and the control that needs it is simply
  // absent when the gateway cannot answer.
  async function loadSupporting() {
    try {
      const response = await fetch(PRESETS_API, { headers: headers() });
      if (response.ok) {
        const data = await response.json();
        schedulePresets = data.presets || [];
      }
    } catch {
      schedulePresets = [];
    }
    try {
      const response = await fetch(USERS_API, { headers: headers() });
      if (response.ok) {
        const data = await response.json();
        users = (data.users || []).map((user) => user.username).filter(Boolean);
      }
    } catch {
      users = [];
    }
    try {
      const response = await fetch(TIMEZONE_API, { headers: headers() });
      if (response.ok) {
        const data = await response.json();
        if (data.Value) timezone = data.Value;
      }
    } catch {
      // The schedule falls back to UTC, which is what the evaluator uses when
      // no time zone is stored.
    }
  }

  async function load() {
    loading = true;
    error = "";
    try {
      const [
        templateResponse,
        groupResponse,
        categoryResponse,
        assignmentResponse,
        deviceResponse,
      ] = await Promise.all([
        fetch(TEMPLATES_API, { headers: headers() }),
        fetch(GROUPS_API, { headers: headers() }),
        fetch(CATEGORIES_API, { headers: headers() }),
        fetch(ASSIGNMENTS_API, { headers: headers() }),
        fetch(DEVICES_API, { headers: headers() }),
        loadSupporting(),
      ]);
      if (!templateResponse.ok) {
        throw new Error(await responseError(templateResponse));
      }
      if (!groupResponse.ok) {
        throw new Error(await responseError(groupResponse));
      }
      if (!categoryResponse.ok) {
        throw new Error(await responseError(categoryResponse));
      }
      if (!assignmentResponse.ok) {
        throw new Error(await responseError(assignmentResponse));
      }
      if (!deviceResponse.ok) {
        throw new Error(await responseError(deviceResponse));
      }
      const templateData = await templateResponse.json();
      const groupData = await groupResponse.json();
      const categoryData = await categoryResponse.json();
      const assignmentData = await assignmentResponse.json();
      const deviceData = await deviceResponse.json();
      templates = templateData.templates || [];
      sharedLimitations = templateData.shared_limitations || [];
      groups = groupData.groups || [];
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

  // Carbon's Tag accepts a fixed set of theme names, so the return type is the
  // subset this page uses rather than a bare string.
  function actionTagType(action: string): "red" | "green" | "gray" {
    if (action === "block") return "red";
    if (action === "allow") return "green";
    return "gray";
  }

  function actionLabel(action: string): string {
    return action || "no action of its own";
  }

  // Svelte re-renders a template expression only when a variable that the
  // expression names changes, so the assignments are grouped into a plain map
  // the template reads directly. A helper that closed over `assignments` would
  // render once and then keep showing stale assignments.
  $: assignedByGroup = assignmentsByGroup(assignments);

  function assignmentsByGroup(list: DeviceAssignment[]): Record<string, string[]> {
    const map: Record<string, string[]> = {};
    for (const assignment of list) {
      (map[assignment.group_id] ||= []).push(assignment.device_id);
    }
    return map;
  }

  // "Who does this apply to?" is the question the group list has to answer,
  // and the answer is the devices assigned to it.
  function assignmentLabel(deviceIDs: string[]): string {
    const count = deviceIDs.length;
    if (!count) return "No devices assigned, so this group changes nothing yet.";
    if (count === 1) return "Enforced on 1 assigned device.";
    return "Enforced on " + count + " assigned devices.";
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

  async function saveGroup() {
    if (!draft.name.trim()) {
      error = "A policy group name is required.";
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
      const data = await response.json();
      const saved = data.group;
      if (isNew) groups = [...groups, saved];
      else groups = groups.map((group) => (group.id === saved.id ? saved : group));
      success = "Policy group saved.";
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
      // The download runs in the background, so the counts may still be the
      // previous ones when this response is written.
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
    if (!confirm("Delete policy group \"" + group.name + "\"? Device assignments to it will be cleared.")) return;
    saving = true;
    error = "";
    try {
      const response = await fetch(GROUPS_API + "/" + group.id, {
        method: "DELETE",
        headers: headers(),
      });
      if (!response.ok) throw new Error(await responseError(response));
      groups = groups.filter((candidate) => candidate.id !== group.id);
      success = "Policy group deleted; assignments to it now use the default policy.";
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
      groups = [...groups, data.group];
      success = template.name + " applied as an editable policy group.";
    } catch (err) {
      error = err.message;
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<Row>
  <Column>
    {#if error}
      <InlineNotification kind="error" title="Error" subtitle={error} on:close={() => (error = "")} />
    {/if}
    {#if success}
      <InlineNotification kind="success" title="Saved" subtitle={success} on:close={() => (success = "")} />
    {/if}

    {#if loading}
      <InlineLoading description="Loading policy templates and groups..." />
    {:else}
      <div class="section-heading">
        <div>
          <h3>Blocked categories</h3>
          <p class="section-intro">
            Domain lists the gateway downloads and keeps current, so no one has
            to type domains. A category selected here is blocked for every
            device, like the global blocklist.
          </p>
        </div>
        <Button size="small" disabled={savingCategories} on:click={saveGatewayCategories}>
          Save categories
        </Button>
      </div>

      <Categoryselect
        {categories}
        selection={gatewaySelection}
        disabled={savingCategories}
        on:change={(event) => (gatewaySelection = event.detail)}
      />

      <div class="section-heading templates-heading">
        <div>
          <h3>Policy templates</h3>
          <p class="section-intro">
            Applying a starter creates an ordinary editable group. Applying it
            again never resets a group you have customized.
          </p>
        </div>
      </div>

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
        <Row>
          {#each templates as template (template.id)}
            <Column sm={4} md={4} lg={4}>
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

                  <Button size="small" disabled={saving || !template.available} on:click={() => applyTemplate(template)}>
                    Apply as editable group
                  </Button>
                </div>
              </Tile>
            </Column>
          {/each}
        </Row>
      </div>

      <div class="section-heading groups-heading">
        <div>
          <h3>Policy groups</h3>
          <p class="section-intro">
            A group holds the categories, domains, and rules for one action, and
            it applies only to the devices you assign to it; every other device
            keeps the gateway default policy. DNS enforces the group decision by
            domain, and a rule that turns on TLS inspection is also enforced by
            the proxy, which is what lets it match URL paths and response types.
          </p>
        </div>
        <Button size="small" kind="secondary" disabled={saving} on:click={beginCreate}>Create policy group</Button>
      </div>

      {#if editingGroupId === "new"}
        <div class="group-slot">
          <Tile>
            <h4>New policy group</h4>
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
          {#if editingGroupId === group.id}
            <Tile>
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
            </Tile>
          {:else}
            <Tile>
              <div class="group-row">
                <div class="group-summary">
                  <div class="group-title">
                    <h4>{group.name}</h4>
                    <Tag type={actionTagType(group.action)}>{actionLabel(group.action)}</Tag>
                  </div>
                  {#if group.description}
                    <p>{group.description}</p>
                  {/if}
                  {#if group.categories?.length}
                    <div class="tags">
                      {#each group.categories as category}
                        <Tag size="sm" type={actionTagType(group.action)}>{categoryName(category)}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if group.domains?.length}
                    <div class="tags">
                      {#each group.domains as domain}
                        <Tag size="sm" type="outline">{domain}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if !group.categories?.length && !group.domains?.length}
                    <p class="muted">No categories or domains selected, so this group matches nothing yet.</p>
                  {/if}

                  <div class="group-rules">
                    <h5>Rules</h5>
                    {#if group.rules?.length}
                      <ul class="rule-list">
                        {#each group.rules as rule (rule.id)}
                          <li>
                            <span class="rule-name">{rule.name || "Untitled rule"}</span>
                            <Tag size="sm" type={actionTagType(rule.action)}>{rule.action}</Tag>
                            {#if !rule.enabled}
                              <Tag size="sm" type="gray">off</Tag>
                            {/if}
                            <span class="rule-conditions">{conditionLabel(rule)}</span>
                          </li>
                        {/each}
                      </ul>
                    {:else}
                      <p class="muted">No rules. The action above applies to the whole group.</p>
                    {/if}
                  </div>

                  <div class="group-assignment">
                    <h5>Assigned devices</h5>
                    <p class="muted">{assignmentLabel(assignedByGroup[group.id] || [])}</p>
                    {#if assignedByGroup[group.id]?.length}
                      <div class="tags">
                        {#each assignedByGroup[group.id] as deviceID (deviceID)}
                          <Tag size="sm">{deviceName(deviceID)}</Tag>
                        {/each}
                      </div>
                    {/if}
                    <!-- Assignment has one home: the device detail on the
                         Devices page. This page only reports who a group
                         applies to, so the two surfaces never disagree. -->
                    <p class="muted">
                      Manage device assignments on the
                      <a href={getBasePath() + "/devices"}>Devices page</a>.
                    </p>
                  </div>
                </div>
                <div class="group-actions">
                  <Button size="small" kind="ghost" disabled={saving} on:click={() => beginEdit(group)}>Edit</Button>
                  <Button size="small" kind="danger-tertiary" disabled={saving} on:click={() => deleteGroup(group)}>Delete</Button>
                </div>
              </div>
            </Tile>
          {/if}
        </div>
      {/each}
    {/if}
  </Column>
</Row>

<style>
  /* This build ships Carbon v10's compiled g10 theme, whose colors are literal
     values rather than --cds-* custom properties, so v10 tokens are used
     directly: #161616 text-01, #525252 text-02. Surfaces come from Carbon's
     tile (#fff over the #f4f4f4 page background), so nothing here draws a
     border or a shadow. */
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
    margin: 0;
    color: #525252;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.32px;
    line-height: 1rem;
  }
  .section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin: 0 0 1rem;
  }
  .templates-heading,
  .groups-heading {
    margin-top: 2rem;
  }
  .section-intro,
  .tile-description,
  .group-summary p,
  .muted {
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  .section-intro {
    margin-bottom: 0;
  }
  /* Each starter carries one caveat. A single muted line reads faster than a
     heading plus a one-item list. */
  .tile-caveat {
    margin: 0 0 0.75rem;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  /* The tiles keep one baseline for the tags and the caveat, so a starter
     without categories still lines up with the rest of the row. */
  .template-tile .tags {
    margin: 0.75rem 0 0.5rem;
  }
  .shared-caveats {
    margin-bottom: 1rem;
  }
  /* The row is a flex container, so stretching the column and filling it from
     the inside keeps every tile in a row the same height. */
  .template-grid :global(.bx--col-sm-4),
  .template-grid :global(.bx--col-md-4),
  .template-grid :global(.bx--col-lg-4) {
    display: flex;
    margin-bottom: 1rem;
  }
  .template-grid :global(.bx--tile) {
    display: flex;
    flex-direction: column;
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
  }
  .group-title {
    display: flex;
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
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  /* The rules of a group are read as one line each: what it is, what it does,
     and the conditions that narrow it. Editing them belongs in the group's own
     form, not in the summary. */
  .group-rules {
    margin-top: 0.75rem;
  }
  .rule-list {
    margin: 0.25rem 0 0;
    padding: 0;
    list-style: none;
  }
  .rule-list li {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 0.5rem;
    padding: 0.25rem 0;
  }
  .rule-name {
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  .rule-conditions {
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  /* Assignment belongs on the card whose rules it applies, so the group list
     answers "who does this apply to?" without a trip to the device page. */
  .group-assignment {
    margin-top: 0.75rem;
  }
  .group-assignment h5 {
    margin-bottom: 0.25rem;
  }
  .group-assignment .muted {
    margin-bottom: 0.5rem;
  }
  .group-actions {
    display: flex;
    align-items: flex-start;
    flex-shrink: 0;
    gap: 0.5rem;
    justify-content: flex-end;
  }
</style>
