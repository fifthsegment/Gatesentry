<script lang="ts">
  import {
    Accordion,
    AccordionItem,
    Button,
    Checkbox,
    Column,
    InlineLoading,
    InlineNotification,
    ListItem,
    Row,
    Select,
    SelectItem,
    Tag,
    TextArea,
    TextInput,
    Tile,
    UnorderedList,
  } from "carbon-components-svelte";
  import { onMount } from "svelte";
  import { getBasePath } from "../../lib/navigate";

  type PolicyTemplate = {
    id: string;
    name: string;
    group_name: string;
    description: string;
    protections: string[];
    limitations: string[];
    categories: string[];
    group_id: string;
    available: boolean;
  };

  type PolicyGroup = {
    id: string;
    name: string;
    description: string;
    domains: string[];
    categories: string[];
    action: string;
    users: string[];
    unknown_device_policy: string;
    priority: number;
  };

  // One self-updating domain feed. The counts come from the last download, so
  // the page can show what a category actually covers.
  type CategoryStatus = {
    id: string;
    name: string;
    description: string;
    domain_count: number;
    updated_at?: string;
    enabled: boolean;
  };

  // A device only leaves the gateway default policy once it is assigned to a
  // group, so the group list shows the assignments that make it effective.
  type DeviceAssignment = {
    device_id: string;
    group_id: string;
  };

  const TEMPLATES_API = getBasePath() + "/api/policy/templates";
  const GROUPS_API = getBasePath() + "/api/policy/groups";
  const CATEGORIES_API = getBasePath() + "/api/policy/categories";
  const ASSIGNMENTS_API = getBasePath() + "/api/policy/assignments";

  let templates: PolicyTemplate[] = [];
  let groups: PolicyGroup[] = [];
  let categories: CategoryStatus[] = [];
  let assignments: DeviceAssignment[] = [];
  let gatewaySelection: string[] = [];
  let loading = true;
  let saving = false;
  let savingCategories = false;
  let error = "";
  let success = "";
  let editingGroupId = "";
  let draft: PolicyGroup = emptyGroup();

  function emptyGroup(): PolicyGroup {
    return {
      id: "",
      name: "",
      description: "",
      domains: [],
      categories: [],
      action: "",
      users: [],
      unknown_device_policy: "",
      priority: 0,
    };
  }

  // Every starter ships the same DNS-scope caveat. State the caveats that all
  // starters share once above the grid instead of repeating identical
  // sentences inside every tile.
  $: sharedLimitations = templates.length
    ? (templates[0].limitations || []).filter((limitation) =>
        templates.every((template) => (template.limitations || []).includes(limitation)),
      )
    : [];

  function ownLimitations(template: PolicyTemplate): string[] {
    return (template.limitations || []).filter(
      (limitation) => !sharedLimitations.includes(limitation),
    );
  }

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

  async function load() {
    loading = true;
    error = "";
    try {
      const [templateResponse, groupResponse, categoryResponse, assignmentResponse] =
        await Promise.all([
          fetch(TEMPLATES_API, { headers: headers() }),
          fetch(GROUPS_API, { headers: headers() }),
          fetch(CATEGORIES_API, { headers: headers() }),
          fetch(ASSIGNMENTS_API, { headers: headers() }),
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
      const templateData = await templateResponse.json();
      const groupData = await groupResponse.json();
      const categoryData = await categoryResponse.json();
      const assignmentData = await assignmentResponse.json();
      templates = templateData.templates || [];
      groups = groupData.groups || [];
      categories = categoryData.categories || [];
      assignments = assignmentData.assignments || [];
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
    draftDomain = "";
  }

  function beginEdit(group: PolicyGroup) {
    editingGroupId = group.id;
    draft = {
      id: group.id,
      name: group.name,
      description: group.description || "",
      domains: [...(group.domains || [])],
      categories: [...(group.categories || [])],
      action: group.action || "",
      users: [...(group.users || [])],
      unknown_device_policy: group.unknown_device_policy || "",
      priority: group.priority || 0,
    };
    draftDomain = "";
  }

  function cancelEdit() {
    editingGroupId = "";
    draft = emptyGroup();
    draftDomain = "";
  }

  function addDomain() {
    const domain = draftDomain.trim();
    if (!domain) return;
    draft.domains = [...draft.domains, domain];
    draftDomain = "";
  }

  function removeDomain(domain: string) {
    draft.domains = draft.domains.filter((candidate) => candidate !== domain);
  }

  // Category checkboxes are driven by an explicit selection array rather than a
  // bound group, so the same helper serves the gateway selection and the group
  // draft and neither can drift from what the API stored.
  function toggleSelection(selection: string[], id: string, checked: boolean): string[] {
    const selected = selection.includes(id);
    if (checked && !selected) return [...selection, id];
    if (!checked && selected) return selection.filter((candidate) => candidate !== id);
    return selection;
  }

  function setDraftCategory(id: string, checked: boolean) {
    draft.categories = toggleSelection(draft.categories, id, checked);
  }

  function setGatewayCategory(id: string, checked: boolean) {
    gatewaySelection = toggleSelection(gatewaySelection, id, checked);
  }

  function categoryName(id: string): string {
    const found = categories.find((category) => category.id === id);
    return found ? found.name : id;
  }

  function coverageLabel(category: CategoryStatus): string {
    if (!category.domain_count) return "Not downloaded yet";
    return category.domain_count.toLocaleString() + " domains";
  }

  // "Who does this apply to?" is the question the group list has to answer,
  // and the answer is the devices assigned to it.
  function assignmentLabel(groupID: string): string {
    const count = assignments.filter((assignment) => assignment.group_id === groupID).length;
    if (!count) return "No devices assigned, so it is not enforced on any device yet. Assign it from the device page.";
    if (count === 1) return "Enforced on 1 assigned device.";
    return "Enforced on " + count + " assigned devices.";
  }

  let draftDomain = "";

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
        body: JSON.stringify({
          id: isNew ? "" : draft.id,
          name: draft.name.trim(),
          description: draft.description,
          domains: draft.domains,
          categories: draft.categories,
          action: draft.action,
          users: draft.users,
          unknown_device_policy: draft.unknown_device_policy,
          priority: draft.priority,
        }),
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

      <div class="category-grid">
        {#each categories as category (category.id)}
          <div class="category-item">
            <div class="category-line">
              <Checkbox
                labelText={category.name}
                checked={gatewaySelection.includes(category.id)}
                disabled={savingCategories}
                on:check={(event) => setGatewayCategory(category.id, event.detail)}
              />
              <span class="category-count">{coverageLabel(category)}</span>
            </div>
            <p class="category-note">{category.description}</p>
          </div>
        {/each}
      </div>

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

                  <h5>Protections</h5>
                  <UnorderedList>
                    {#each template.protections as protection}
                      <ListItem>{protection}</ListItem>
                    {/each}
                  </UnorderedList>

                  {#if ownLimitations(template).length}
                    <h5>Limitations</h5>
                    <UnorderedList>
                      {#each ownLimitations(template) as limitation}
                        <ListItem>{limitation}</ListItem>
                      {/each}
                    </UnorderedList>
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
          <h3>Editable policy groups</h3>
          <p class="section-intro">
            A group is a list of categories and domains plus one action. It
            applies only to the devices you assign to it; every other device
            keeps the gateway default policy.
          </p>
        </div>
        <Button size="small" kind="secondary" disabled={saving} on:click={beginCreate}>Create policy group</Button>
      </div>

      {#if editingGroupId === "new"}
        <div class="group-slot">
          <Tile>
            <h4>New policy group</h4>
            <TextInput labelText="Name" bind:value={draft.name} />
            <TextArea labelText="Description" bind:value={draft.description} rows={2} />
            <Select labelText="Domain action" bind:selected={draft.action}>
              <SelectItem value="" text="No additional action" />
              <SelectItem value="block" text="Block the selected categories and domains" />
              <SelectItem value="allow" text="Allow the selected categories and domains" />
            </Select>
            <fieldset class="category-fieldset">
              <legend class="category-legend">Categories</legend>
              <div class="category-grid">
                {#each categories as category (category.id)}
                  <div class="category-item">
                    <div class="category-line">
                      <Checkbox
                        labelText={category.name}
                        checked={draft.categories.includes(category.id)}
                        on:check={(event) => setDraftCategory(category.id, event.detail)}
                      />
                      <span class="category-count">{coverageLabel(category)}</span>
                    </div>
                    <p class="category-note">{category.description}</p>
                  </div>
                {/each}
              </div>
            </fieldset>
            <Accordion>
              <AccordionItem title="Specific domains">
                <div class="domain-entry">
                  <TextInput labelText="Domain pattern" placeholder="example.com or *.example.com" bind:value={draftDomain} on:keydown={(event) => event.key === "Enter" && addDomain()} />
                  <Button size="small" kind="tertiary" on:click={addDomain}>Add domain</Button>
                </div>
                {#if draft.domains.length}
                  <div class="domain-tags">
                    {#each draft.domains as domain}
                      <Tag filter size="sm" on:close={() => removeDomain(domain)}>{domain}</Tag>
                    {/each}
                  </div>
                {/if}
              </AccordionItem>
            </Accordion>
            <div class="group-actions">
              <Button size="small" disabled={saving} on:click={saveGroup}>Save group</Button>
              <Button size="small" kind="ghost" on:click={cancelEdit}>Cancel</Button>
            </div>
          </Tile>
        </div>
      {/if}

      {#each groups as group (group.id)}
        <div class="group-slot">
          {#if editingGroupId === group.id}
            <Tile>
              <h4>Edit {group.name}</h4>
              <TextInput labelText="Name" bind:value={draft.name} />
              <TextArea labelText="Description" bind:value={draft.description} rows={2} />
              <Select labelText="Domain action" bind:selected={draft.action}>
                <SelectItem value="" text="No additional action" />
                <SelectItem value="block" text="Block the selected categories and domains" />
                <SelectItem value="allow" text="Allow the selected categories and domains" />
              </Select>
              <fieldset class="category-fieldset">
                <legend class="category-legend">Categories</legend>
                <div class="category-grid">
                  {#each categories as category (category.id)}
                    <div class="category-item">
                      <div class="category-line">
                        <Checkbox
                          labelText={category.name}
                          checked={draft.categories.includes(category.id)}
                          on:check={(event) => setDraftCategory(category.id, event.detail)}
                        />
                        <span class="category-count">{coverageLabel(category)}</span>
                      </div>
                      <p class="category-note">{category.description}</p>
                    </div>
                  {/each}
                </div>
              </fieldset>
              <Accordion>
                <AccordionItem title="Specific domains">
                  <div class="domain-entry">
                    <TextInput labelText="Domain pattern" placeholder="example.com or *.example.com" bind:value={draftDomain} on:keydown={(event) => event.key === "Enter" && addDomain()} />
                    <Button size="small" kind="tertiary" on:click={addDomain}>Add domain</Button>
                  </div>
                  {#if draft.domains.length}
                    <div class="domain-tags">
                      {#each draft.domains as domain}
                        <Tag filter size="sm" on:close={() => removeDomain(domain)}>{domain}</Tag>
                      {/each}
                    </div>
                  {/if}
                </AccordionItem>
              </Accordion>
              <div class="group-actions">
                <Button size="small" disabled={saving} on:click={saveGroup}>Save group</Button>
                <Button size="small" kind="ghost" on:click={cancelEdit}>Cancel</Button>
              </div>
            </Tile>
          {:else}
            <Tile>
              <div class="group-row">
                <div class="group-summary">
                  <div class="group-title">
                    <h4>{group.name}</h4>
                    <Tag type={group.action === "block" ? "red" : group.action === "allow" ? "green" : "gray"}>
                      {group.action || "default behavior"}
                    </Tag>
                  </div>
                  <p>{group.description || "No description."}</p>
                  {#if group.categories?.length}
                    {@const categoryTagType = group.action === "block" ? "red" : group.action === "allow" ? "green" : "gray"}
                    <div class="domain-tags">
                      {#each group.categories as category}
                        <Tag size="sm" type={categoryTagType}>{categoryName(category)}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if group.domains?.length}
                    <div class="domain-tags">
                      {#each group.domains as domain}
                        <Tag size="sm" type="outline">{domain}</Tag>
                      {/each}
                    </div>
                  {/if}
                  {#if !group.categories?.length && !group.domains?.length}
                    <p class="muted">No categories or domains selected. Assigned devices use the gateway default policy.</p>
                  {/if}
                  <p class="muted">{assignmentLabel(group.id)}</p>
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
    margin: 1rem 0 0;
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
  /* Carbon's unordered list hangs its en dash one rem to the left of the item
     text, so the list needs an inset to keep the dashes inside the tile. */
  .template-tile :global(.bx--list--unordered) {
    margin: 0.25rem 0 0;
    padding-left: 1rem;
  }
  .template-tile :global(.bx--list__item) {
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  .shared-caveats {
    margin-bottom: 1rem;
  }
  /* A checkbox group needs the same label treatment as a single field, and a
     fieldset/legend pair keeps the group's name associated with its boxes. */
  .category-fieldset {
    margin: 0;
    padding: 0;
    border: 0;
  }
  .category-legend {
    margin-bottom: 0.5rem;
    color: #161616;
    font-size: 0.75rem;
    font-weight: 400;
    letter-spacing: 0.32px;
    line-height: 1rem;
  }
  /* The entries are short, so a laptop fits two or three columns instead of
     one long list. */
  .category-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
    column-gap: 2rem;
  }
  .category-line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
  }
  /* Carbon's form item carries a 1rem bottom margin, which is taller than the
     dense selection list needs. This rule is more specific than the group-slot
     form-item margin below, so the order of these blocks does not matter. */
  .category-grid .category-item :global(.bx--form-item) {
    margin-bottom: 0;
  }
  /* The description says what a category covers and, more usefully, what it
     leaves alone, so it sits under the box instead of in a tooltip. */
  .category-note {
    margin: 0 0 0.75rem;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  .category-count {
    flex-shrink: 0;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
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
  .group-slot :global(.bx--form-item) {
    margin-bottom: 0.75rem;
  }
  .group-slot :global(.bx--accordion) {
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
  .domain-tags :global(.bx--tag) {
    margin: 0;
  }
  .domain-entry {
    display: flex;
    align-items: flex-end;
    gap: 1rem;
  }
  .domain-entry :global(.bx--text-input-wrapper) {
    flex: 1;
  }
  .domain-entry :global(.bx--form-item) {
    margin-bottom: 0;
  }
  .domain-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  .group-actions {
    display: flex;
    align-items: center;
    flex-shrink: 0;
    gap: 0.5rem;
    justify-content: flex-end;
  }
</style>
