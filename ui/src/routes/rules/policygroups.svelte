<script lang="ts">
  import {
    Button,
    Column,
    Grid,
    InlineLoading,
    InlineNotification,
    Row,
    Select,
    SelectItem,
    Tag,
    TextArea,
    TextInput,
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
        <p>
          Review the protections and limitations first. Applying a template
          creates an ordinary editable group; applying it again never resets a
          group you have customized.
        </p>
      </div>
      <Tag type="blue">Advanced rule editor remains below</Tag>
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
      <Grid condensed>
        {#each templates as template (template.id)}
          <Column sm={4} md={4} lg={5} class="template-column">
            <article class="template-card">
              <h4>{template.name}</h4>
              <p>{template.description}</p>
              <h5>Before applying</h5>
              <strong>Protections</strong>
              <ul>
                {#each template.protections as protection}
                  <li>{protection}</li>
                {/each}
              </ul>
              <strong>Limitations</strong>
              <ul class="limitations">
                {#each template.limitations as limitation}
                  <li>{limitation}</li>
                {/each}
              </ul>
              <Button size="small" disabled={saving || !template.available} on:click={() => applyTemplate(template)}>
                Apply as editable group
              </Button>
            </article>
          </Column>
        {/each}
      </Grid>

      <div class="groups-heading">
        <div>
          <h3>Editable policy groups</h3>
          <p>These records drive explicit group assignment. Owner and category labels do not create policy rules.</p>
        </div>
        <Button size="small" kind="secondary" disabled={saving} on:click={beginCreate}>Create policy group</Button>
      </div>

      {#if editingGroupId === "new"}
        <div class="group-editor">
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
        </div>
      {/if}

      {#each groups as group (group.id)}
        <div class="group-row">
          {#if editingGroupId === group.id}
            <div class="group-editor">
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
            </div>
          {:else}
            <div class="group-summary">
              <h4>{group.name}</h4>
              <Tag type={group.action === "block" ? "red" : group.action === "allow" ? "green" : "gray"}>
                {group.action || "default behavior"}
              </Tag>
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
          {/if}
        </div>
      {/each}
    {/if}
  </Column>
</Row>

<style>
  .section-heading,
  .groups-heading,
  .group-row,
  .domain-entry,
  .group-actions {
    display: flex;
    align-items: flex-start;
    gap: 1rem;
  }
  .section-heading,
  .groups-heading {
    justify-content: space-between;
    margin: 1rem 0;
  }
  h3,
  h4,
  h5,
  p {
    margin-top: 0;
  }
  .section-heading p,
  .groups-heading p,
  .template-card p,
  .group-summary p,
  .muted {
    color: #525252;
    font-size: 0.875rem;
  }
  .template-card,
  .group-row,
  .group-editor {
    border: 1px solid #c6c6c6;
    padding: 1rem;
    height: 100%;
    box-sizing: border-box;
  }
  .template-card h4 {
    margin-bottom: 0.5rem;
  }
  .template-card h5 {
    margin: 1rem 0 0.35rem;
  }
  .template-card ul {
    padding-left: 1.25rem;
    font-size: 0.8125rem;
  }
  .template-card .limitations {
    color: #525252;
  }
  .groups-heading {
    margin-top: 2rem;
  }
  .group-row {
    justify-content: space-between;
    margin-bottom: 0.75rem;
  }
  .group-summary {
    flex: 1;
  }
  .group-summary h4 {
    display: inline-block;
    margin-right: 0.5rem;
  }
  .group-editor {
    width: 100%;
  }
  .group-editor :global(.bx--text-input-wrapper),
  .group-editor :global(.bx--text-area__wrapper),
  .group-editor :global(.bx--select) {
    margin-bottom: 0.75rem;
  }
  .domain-entry {
    align-items: flex-end;
  }
  .domain-entry :global(.bx--text-input-wrapper) {
    flex: 1;
  }
  .domain-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  .group-actions {
    align-items: center;
    justify-content: flex-end;
    flex-shrink: 0;
  }
</style>
