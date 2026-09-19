<script lang="ts">
  import {
    Button,
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
    group_id: string;
    available: boolean;
  };

  type PolicyGroup = {
    id: string;
    name: string;
    description: string;
    domains: string[];
    action: string;
    users: string[];
    unknown_device_policy: string;
    priority: number;
  };

  const TEMPLATES_API = getBasePath() + "/api/policy/templates";
  const GROUPS_API = getBasePath() + "/api/policy/groups";

  let templates: PolicyTemplate[] = [];
  let groups: PolicyGroup[] = [];
  let loading = true;
  let saving = false;
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
      const [templateResponse, groupResponse] = await Promise.all([
        fetch(TEMPLATES_API, { headers: headers() }),
        fetch(GROUPS_API, { headers: headers() }),
      ]);
      if (!templateResponse.ok) {
        throw new Error(await responseError(templateResponse));
      }
      if (!groupResponse.ok) {
        throw new Error(await responseError(groupResponse));
      }
      const templateData = await templateResponse.json();
      const groupData = await groupResponse.json();
      templates = templateData.templates || [];
      groups = groupData.groups || [];
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
    <div class="section-heading">
      <div>
        <h3>Policy templates</h3>
        <p class="section-intro">
          Applying a starter creates an ordinary editable group. Applying it
          again never resets a group you have customized.
        </p>
      </div>
    </div>

    {#if error}
      <InlineNotification kind="error" title="Error" subtitle={error} on:close={() => (error = "")} />
    {/if}
    {#if success}
      <InlineNotification kind="success" title="Saved" subtitle={success} on:close={() => (success = "")} />
    {/if}

    {#if loading}
      <InlineLoading description="Loading policy templates and groups..." />
    {:else}
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
            These records drive explicit group assignment. A name or category
            label does not create a rule.
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
              <SelectItem value="block" text="Block listed domains" />
              <SelectItem value="allow" text="Allow listed domains" />
            </Select>
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
                <SelectItem value="block" text="Block listed domains" />
                <SelectItem value="allow" text="Allow listed domains" />
              </Select>
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
                  {#if group.domains?.length}
                    <div class="domain-tags">
                      {#each group.domains as domain}
                        <Tag size="sm" type="outline">{domain}</Tag>
                      {/each}
                    </div>
                  {:else}
                    <p class="muted">No explicit domain patterns. The gateway default policy remains in effect.</p>
                  {/if}
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
