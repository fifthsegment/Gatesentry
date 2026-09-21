<script lang="ts">
  import {
    Button,
    Column,
    MultiSelect,
    Row,
    Select,
    SelectItem,
    Tag,
    TextArea,
    TextInput,
    TimePicker,
    Toggle,
  } from "carbon-components-svelte";
  import { ChevronDown, ChevronUp } from "carbon-icons-svelte";
  import Categoryselect from "./categoryselect.svelte";
  import { MITM_DEFAULT, SCHEDULE_PRESET_CUSTOM, WEEKDAYS, emptyRule, scheduleLabel } from "./policymodel";
  import type { CategoryStatus, GroupRule, PolicyGroup, SchedulePreset } from "./policymodel";

  // The draft is the page's object, mutated in place: the page reads it back
  // when it saves, so the form never owns a second copy of the policy.
  export let draft: PolicyGroup;
  export let categories: CategoryStatus[] = [];
  export let users: string[] = [];
  export let schedulePresets: SchedulePreset[] = [];
  export let timezone = "UTC";
  export let saving = false;
  export let onSave: () => void = () => {};
  export let onCancel: () => void = () => {};

  // 24-hour wall clock, which is what a schedule stores. Carbon's default
  // pattern accepts 12-hour input only.
  const TIME_PATTERN = "([01][0-9]|2[0-3]):[0-5][0-9]";

  let draftDomain = "";

  // Svelte re-renders an each block when the array it reads is reassigned, and
  // the rule objects keep their identity, so the page still sees every edit.
  function touchRules() {
    draft.rules = [...draft.rules];
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

  function addRule() {
    draft.rules = [...draft.rules, emptyRule()];
  }

  function removeRule(index: number) {
    draft.rules = draft.rules.filter((_, position) => position !== index);
  }

  // Rule order is evaluation order: the first matching rule decides. Moving a
  // rule is how an administrator changes which one wins.
  function moveRule(index: number, offset: number) {
    const target = index + offset;
    if (target < 0 || target >= draft.rules.length) return;
    const next = [...draft.rules];
    [next[index], next[target]] = [next[target], next[index]];
    draft.rules = next;
  }

  // URL and response-type conditions are tested inside a decrypted request, so
  // only a block rule that keeps inspection on can carry them. The API rejects
  // the other combinations, and the form turns these fields off rather than
  // letting an administrator write a rule the gateway would refuse.
  function conditionsAllowed(rule: GroupRule): boolean {
    return rule.action === "block" && rule.mitm_action !== "disable";
  }

  function addUrlPattern(rule: GroupRule) {
    const pattern = rule.url_draft.trim();
    if (!pattern) return;
    rule.url_regexes = [...rule.url_regexes, pattern];
    rule.url_draft = "";
    touchRules();
  }

  function removeUrlPattern(rule: GroupRule, pattern: string) {
    rule.url_regexes = rule.url_regexes.filter((candidate) => candidate !== pattern);
    touchRules();
  }

  function addContentType(rule: GroupRule) {
    const contentType = rule.content_type_draft.trim().toLowerCase();
    if (!contentType) return;
    rule.blocked_content_types = [...rule.blocked_content_types, contentType];
    rule.content_type_draft = "";
    touchRules();
  }

  function removeContentType(rule: GroupRule, contentType: string) {
    rule.blocked_content_types = rule.blocked_content_types.filter((candidate) => candidate !== contentType);
    touchRules();
  }

  function setRuleUsers(rule: GroupRule, selected: (string | number)[]) {
    rule.users = selected.map(String);
    touchRules();
  }

  function setWeekdays(rule: GroupRule, selected: (string | number)[]) {
    if (!rule.schedule) return;
    rule.schedule.weekdays = selected.map(Number).sort((a, b) => a - b);
    touchRules();
  }

  // The select value is derived from the schedule, so a preset picked here is
  // stored as the preset's own windows and days rather than as a name the
  // evaluator would have to look up.
  function scheduleChoice(rule: GroupRule): string {
    if (!rule.schedule) return "";
    return rule.schedule.preset || SCHEDULE_PRESET_CUSTOM;
  }

  function setScheduleChoice(rule: GroupRule, choice: string) {
    if (!choice) {
      rule.schedule = null;
      touchRules();
      return;
    }
    if (choice === SCHEDULE_PRESET_CUSTOM) {
      const window = rule.schedule?.windows?.[0] || { from: "20:00", to: "07:00" };
      rule.schedule = {
        timezone,
        weekdays: rule.schedule?.weekdays ? [...rule.schedule.weekdays] : [],
        windows: [{ from: window.from, to: window.to }],
      };
      touchRules();
      return;
    }
    const preset = schedulePresets.find((candidate) => candidate.id === choice);
    if (!preset) return;
    rule.schedule = {
      timezone,
      weekdays: [...preset.weekdays],
      windows: preset.windows.map((window) => ({ from: window.from, to: window.to })),
      preset: preset.id,
    };
    touchRules();
  }

  function setWindowTime(rule: GroupRule, key: "from" | "to", value: string) {
    if (!rule.schedule?.windows?.length) return;
    rule.schedule.windows[0][key] = value;
    touchRules();
  }

  // The controls report a DOM event, and Svelte's template parser does not
  // accept a type assertion inside an inline handler, so each one reads its
  // value through a named function.
  function onScheduleChange(rule: GroupRule, event: Event) {
    setScheduleChoice(rule, (event.target as HTMLSelectElement).value);
  }

  function onWindowTimeChange(rule: GroupRule, key: "from" | "to", event: Event) {
    setWindowTime(rule, key, (event.target as HTMLInputElement).value);
  }

  function presetLabel(preset: SchedulePreset): string {
    const window = preset.windows[0];
    if (!window) return preset.name;
    return preset.name + " (" + window.from + "-" + window.to + ")";
  }
</script>

<Row>
  <Column sm={4} md={8} lg={8}>
    <TextInput labelText="Name" bind:value={draft.name} />
  </Column>
  <Column sm={4} md={8} lg={8}>
    <Select labelText="Action on the selected categories and domains" bind:selected={draft.action}>
      <SelectItem value="" text="No action of its own" />
      <SelectItem value="block" text="Block" />
      <SelectItem value="allow" text="Allow" />
    </Select>
  </Column>
</Row>
<Row>
  <Column sm={4} md={8} lg={16}>
    <TextArea labelText="Description" rows={2} bind:value={draft.description} />
  </Column>
</Row>

<fieldset class="field">
  <legend class="field-label">Categories</legend>
  <Categoryselect {categories} selection={draft.categories} on:change={(event) => (draft.categories = event.detail)} />
</fieldset>

<fieldset class="field">
  <legend class="field-label">Domains</legend>
  <div class="list-entry">
    <TextInput
      labelText="Add a domain pattern"
      placeholder="example.com or *.example.com"
      bind:value={draftDomain}
      on:keydown={(event) => event.key === "Enter" && addDomain()}
    />
    <Button size="small" kind="tertiary" on:click={addDomain}>Add domain</Button>
  </div>
  {#if draft.domains.length}
    <div class="tags">
      {#each draft.domains as domain (domain)}
        <Tag filter size="sm" on:close={() => removeDomain(domain)}>{domain}</Tag>
      {/each}
    </div>
  {/if}
</fieldset>

<fieldset class="field">
  <legend class="field-label">Rules</legend>
  <p class="field-note">
    Rules run top to bottom and the first match decides. URL and response-type
    conditions only narrow a block rule and need TLS inspection, so they are off
    for every other rule.
  </p>

  {#each draft.rules as rule, index (index)}
    <div class="rule">
      <div class="rule-head">
        <h5>Rule {index + 1}</h5>
        <div class="rule-head-controls">
          <Toggle size="sm" labelText="Enabled" bind:toggled={rule.enabled} />
          <Button
            size="small"
            kind="ghost"
            icon={ChevronUp}
            iconDescription="Move rule up"
            disabled={index === 0}
            on:click={() => moveRule(index, -1)}
          />
          <Button
            size="small"
            kind="ghost"
            icon={ChevronDown}
            iconDescription="Move rule down"
            disabled={index === draft.rules.length - 1}
            on:click={() => moveRule(index, 1)}
          />
          <Button size="small" kind="danger-ghost" on:click={() => removeRule(index)}>Remove rule</Button>
        </div>
      </div>

      <Row>
        <Column sm={4} md={8} lg={6}>
          <TextInput labelText="Name" placeholder="Bedtime social block" bind:value={rule.name} />
        </Column>
        <Column sm={4} md={4} lg={5}>
          <Select labelText="Rule action" bind:selected={rule.action}>
            <SelectItem value="block" text="Block" />
            <SelectItem value="allow" text="Allow (exception)" />
          </Select>
        </Column>
        <Column sm={4} md={4} lg={5}>
          <Select labelText="TLS inspection" bind:selected={rule.mitm_action}>
            <SelectItem value={MITM_DEFAULT} text="Gateway setting" />
            <SelectItem value="enable" text="Inspect this rule's traffic" />
            <SelectItem value="disable" text="Never inspect it" />
          </Select>
        </Column>
      </Row>

      <Row>
        <Column sm={4} md={8} lg={8}>
          <div class="list-entry">
            <TextInput
              labelText="URL pattern"
              placeholder="/watch"
              disabled={!conditionsAllowed(rule)}
              bind:value={rule.url_draft}
              on:keydown={(event) => event.key === "Enter" && addUrlPattern(rule)}
            />
            <Button size="small" kind="tertiary" disabled={!conditionsAllowed(rule)} on:click={() => addUrlPattern(rule)}>
              Add pattern
            </Button>
          </div>
          {#if rule.url_regexes.length}
            <div class="tags">
              {#each rule.url_regexes as pattern (pattern)}
                <Tag filter size="sm" on:close={() => removeUrlPattern(rule, pattern)}>{pattern}</Tag>
              {/each}
            </div>
          {/if}
        </Column>
        <Column sm={4} md={8} lg={8}>
          <div class="list-entry">
            <TextInput
              labelText="Response type"
              placeholder="video/"
              disabled={!conditionsAllowed(rule)}
              bind:value={rule.content_type_draft}
              on:keydown={(event) => event.key === "Enter" && addContentType(rule)}
            />
            <Button size="small" kind="tertiary" disabled={!conditionsAllowed(rule)} on:click={() => addContentType(rule)}>
              Add type
            </Button>
          </div>
          {#if rule.blocked_content_types.length}
            <div class="tags">
              {#each rule.blocked_content_types as contentType (contentType)}
                <Tag filter size="sm" on:close={() => removeContentType(rule, contentType)}>{contentType}</Tag>
              {/each}
            </div>
          {/if}
        </Column>
      </Row>

      <Row>
        <Column sm={4} md={8} lg={8}>
          <Select
            labelText="Active hours"
            selected={scheduleChoice(rule)}
            helperText={rule.schedule ? scheduleLabel(rule.schedule) : ""}
            on:change={(event) => onScheduleChange(rule, event)}
          >
            <SelectItem value="" text="Always active" />
            {#each schedulePresets as preset (preset.id)}
              <SelectItem value={preset.id} text={presetLabel(preset)} />
            {/each}
            <SelectItem value={SCHEDULE_PRESET_CUSTOM} text="Custom hours" />
          </Select>
          {#if rule.schedule && !rule.schedule.preset}
            <div class="window-row">
              <TimePicker
                size="sm"
                labelText="From"
                placeholder="HH:MM"
                pattern={TIME_PATTERN}
                value={rule.schedule.windows[0]?.from || ""}
                on:change={(event) => onWindowTimeChange(rule, "from", event)}
              />
              <TimePicker
                size="sm"
                labelText="To"
                placeholder="HH:MM"
                pattern={TIME_PATTERN}
                value={rule.schedule.windows[0]?.to || ""}
                on:change={(event) => onWindowTimeChange(rule, "to", event)}
              />
            </div>
            <MultiSelect
              titleText="Days"
              label="Every day"
              items={WEEKDAYS}
              selectedIds={rule.schedule.weekdays.map(String)}
              on:select={(event) => setWeekdays(rule, event.detail.selectedIds)}
            />
          {/if}
        </Column>
        <Column sm={4} md={8} lg={8}>
          {#if users.length}
            <MultiSelect
              titleText="Applies to users"
              label="Any user in this group"
              items={users.map((user) => ({ id: user, text: user }))}
              selectedIds={rule.users}
              on:select={(event) => setRuleUsers(rule, event.detail.selectedIds)}
            />
          {/if}
        </Column>
      </Row>
    </div>
  {/each}

  <Button size="small" kind="tertiary" on:click={addRule}>Add rule</Button>
</fieldset>

<div class="group-actions">
  <Button size="small" disabled={saving} on:click={onSave}>Save group</Button>
  <Button size="small" kind="ghost" on:click={onCancel}>Cancel</Button>
</div>

<style>
  /* Carbon v10's compiled g10 theme exposes literal colors rather than --cds-*
     custom properties: #161616 text-01, #525252 text-02, and #f4f4f4 for the
     nested rule surface under a white tile. Nothing here draws a border or a
     shadow of its own. */
  h5,
  p {
    margin-top: 0;
  }
  h5 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.375rem;
  }
  /* A fieldset names a repeated control the way Carbon's own form groups do:
     the legend is styled as a field label, not as a heading. */
  .field {
    margin: 1.5rem 0 0;
    padding: 0;
    border: 0;
  }
  .field-label {
    margin-bottom: 0.5rem;
    color: #161616;
    font-size: 0.75rem;
    font-weight: 400;
    letter-spacing: 0.32px;
    line-height: 1rem;
  }
  .field-note {
    margin: 0 0 0.75rem;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  .rule {
    margin-bottom: 0.75rem;
    padding: 1rem;
    background: #f4f4f4;
  }
  .rule-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 0.5rem;
  }
  .rule-head-controls {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  /* The entry field and its Add button share one line, and the button lines up
     with the input rather than with its label. */
  .list-entry {
    display: flex;
    align-items: flex-end;
    gap: 1rem;
  }
  .list-entry :global(.bx--text-input-wrapper) {
    flex: 1;
  }
  .list-entry :global(.bx--form-item) {
    margin-bottom: 0;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  .tags :global(.bx--tag) {
    margin: 0;
  }
  .window-row {
    display: flex;
    gap: 1rem;
    margin-bottom: 0.75rem;
  }
  .group-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
