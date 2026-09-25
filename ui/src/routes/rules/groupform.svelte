<script lang="ts">
  import {
    Accordion,
    AccordionItem,
    Button,
    Column,
    FormGroup,
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
  import Domainlist from "./domainlist.svelte";
  import {
    DEFAULT_GROUP_ID,
    SCHEDULE_PRESET_CUSTOM,
    WEEKDAYS,
    bedtimeRule,
    emptyRule,
    hasAdvancedSettings,
    isBedtimeRule,
    needsProxy,
    scheduleLabel,
  } from "./policymodel";
  import type {
    CategoryStatus,
    GroupRule,
    PolicyGroup,
    SchedulePreset,
  } from "./policymodel";

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

  $: isDefault = draft.id === DEFAULT_GROUP_ID;

  // The simple editor covers what most households need: categories, sites,
  // safe search, and bedtime. Everything else lives under Advanced, which
  // opens on its own when the policy already uses it.
  const startAdvanced = hasAdvancedSettings(draft);
  $: bedtimeIndex = draft.rules.findIndex(isBedtimeRule);
  $: bedtime = bedtimeIndex >= 0 ? draft.rules[bedtimeIndex] : null;

  function setBedtime(on: boolean) {
    if (on && !bedtime) {
      const preset = schedulePresets.find(
        (candidate) => candidate.id === "bedtime",
      );
      draft.rules = [bedtimeRule(timezone, preset), ...draft.rules];
    } else if (!on && bedtime) {
      removeRule(bedtimeIndex);
    }
  }

  function onBedtimeTime(key: "from" | "to", event: Event) {
    if (!bedtime?.schedule) return;
    bedtime.schedule.windows[0][key] = (event.target as HTMLInputElement).value;
    bedtime.schedule.preset = undefined;
    touchRules();
  }

  function onBedtimeDays(selected: (string | number)[]) {
    if (!bedtime?.schedule) return;
    bedtime.schedule.weekdays = selected.map(Number).sort((a, b) => a - b);
    bedtime.schedule.preset = undefined;
    touchRules();
  }

  // Svelte re-renders an each block when the array it reads is reassigned, and
  // the rule objects keep their identity, so the page still sees every edit.
  function touchRules() {
    draft.rules = [...draft.rules];
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

  // URL and response-type conditions narrow a block to part of a site, so an
  // allow rule cannot carry them. The API rejects the combination, and the
  // form turns the fields off rather than letting an administrator write it.
  function conditionsAllowed(rule: GroupRule): boolean {
    return rule.action === "block";
  }

  function setTargetKind(rule: GroupRule, event: Event) {
    rule.target.all_traffic =
      (event.target as HTMLSelectElement).value === "all";
    touchRules();
  }

  function addListEntry(
    rule: GroupRule,
    field: "url_regexes" | "blocked_content_types",
    draftField: "url_draft" | "content_type_draft",
    lower = false,
  ) {
    let value = rule[draftField].trim();
    if (lower) value = value.toLowerCase();
    if (!value || rule[field].includes(value)) return;
    rule[field] = [...rule[field], value];
    rule[draftField] = "";
    touchRules();
  }

  function removeListEntry(
    rule: GroupRule,
    field: "url_regexes" | "blocked_content_types",
    value: string,
  ) {
    rule[field] = rule[field].filter((candidate) => candidate !== value);
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
      const window = rule.schedule?.windows?.[0] || {
        from: "20:00",
        to: "07:00",
      };
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
      windows: preset.windows.map((window) => ({
        from: window.from,
        to: window.to,
      })),
      preset: preset.id,
    };
    touchRules();
  }

  // The controls report a DOM event, and Svelte's template parser does not
  // accept a type assertion inside an inline handler, so each one reads its
  // value through a named function.
  function onScheduleChange(rule: GroupRule, event: Event) {
    setScheduleChoice(rule, (event.target as HTMLSelectElement).value);
  }

  function onWindowTimeChange(
    rule: GroupRule,
    key: "from" | "to",
    event: Event,
  ) {
    if (!rule.schedule?.windows?.length) return;
    rule.schedule.windows[0][key] = (event.target as HTMLInputElement).value;
    touchRules();
  }

  function presetLabel(preset: SchedulePreset): string {
    const window = preset.windows[0];
    if (!window) return preset.name;
    return preset.name + " (" + window.from + "-" + window.to + ")";
  }

  function setGroupUsers(selected: (string | number)[]) {
    draft.users = selected.map(String);
  }
</script>

<form class="policy-form" on:submit|preventDefault={onSave}>
  <div class="form-section">
    <FormGroup legendText="Basic protection">
      <p class="section-description">
        Name the policy, choose broad categories, and add site exceptions.
      </p>
      <div class="name-field">
        <TextInput
          labelText="Name"
          placeholder="Kids"
          bind:value={draft.name}
        />
      </div>

      <fieldset class="field">
        <legend class="field-label">Blocked categories</legend>
        <p class="field-note">Lists of sites that update themselves.</p>
        <Categoryselect
          {categories}
          selection={draft.blocked_categories}
          on:change={(event) => (draft.blocked_categories = event.detail)}
        />
      </fieldset>

      <Row>
        <Column sm={4} md={4} lg={8}>
          <fieldset class="field">
            <legend class="field-label">Blocked domains</legend>
            <Domainlist
              label="Block a domain"
              entries={draft.blocked_domains}
              tagType="red"
              on:change={(event) => (draft.blocked_domains = event.detail)}
            />
          </fieldset>
        </Column>
        <Column sm={4} md={4} lg={8}>
          <fieldset class="field">
            <legend class="field-label">Always allowed domains</legend>
            <Domainlist
              label="Allow a domain"
              entries={draft.allowed_domains}
              tagType="green"
              on:change={(event) => (draft.allowed_domains = event.detail)}
            />
            <p class="field-note">
              Always reachable, even if a blocked category lists it.
            </p>
          </fieldset>
        </Column>
      </Row>

      <div class="field">
        <Toggle
          labelText="Safe search"
          labelA="Off"
          labelB="Google, Bing, DuckDuckGo, and YouTube restricted mode"
          bind:toggled={draft.safe_search}
        />
      </div>
    </FormGroup>
  </div>

  <div class="form-section">
    <FormGroup legendText="Schedule">
      <p class="section-description">
        Optionally pause internet access overnight in the gateway time zone.
      </p>
      <Toggle
        labelText="Bedtime"
        labelA="Off"
        labelB="No internet during these hours"
        toggled={!!bedtime}
        on:toggle={(event) => setBedtime(event.detail.toggled)}
      />
      {#if bedtime?.schedule}
        <div class="window-row bedtime-row">
          <TimePicker
            size="sm"
            labelText="From"
            placeholder="HH:MM"
            pattern={TIME_PATTERN}
            value={bedtime.schedule.windows[0]?.from || ""}
            on:change={(event) => onBedtimeTime("from", event)}
          />
          <TimePicker
            size="sm"
            labelText="Until"
            placeholder="HH:MM"
            pattern={TIME_PATTERN}
            value={bedtime.schedule.windows[0]?.to || ""}
            on:change={(event) => onBedtimeTime("to", event)}
          />
          <MultiSelect
            size="sm"
            titleText="Nights"
            label="Every night"
            items={WEEKDAYS}
            selectedIds={bedtime.schedule.weekdays.map(String)}
            on:select={(event) => onBedtimeDays(event.detail.selectedIds)}
          />
        </div>
        <p class="field-note">
          A night lasts until the next morning, in {bedtime.schedule.timezone ||
            "UTC"}.
        </p>
      {/if}
    </FormGroup>
  </div>

  <div class="form-section advanced-form-section">
    <FormGroup legendText="Advanced rules">
      <p class="section-description">
        Use only when basic category, domain, safe-search, and bedtime controls
        are not enough.
      </p>
      <Accordion align="start">
        <AccordionItem
          title="Custom rules, proxy users, and description"
          open={startAdvanced}
        >
          <div class="advanced-content">
            <div class="field first-field">
              <TextArea
                labelText="Description"
                rows={2}
                bind:value={draft.description}
              />
            </div>

            {#if !isDefault && users.length}
              <div class="field">
                <MultiSelect
                  titleText="Proxy users on this policy"
                  label="Only devices assigned to it"
                  items={users.map((user) => ({ id: user, text: user }))}
                  selectedIds={draft.users}
                  on:select={(event) => setGroupUsers(event.detail.selectedIds)}
                />
                <p class="field-note">
                  A signed-in proxy user gets this policy on any device.
                </p>
              </div>
            {/if}

            <fieldset class="field">
              <legend class="field-label">Rules</legend>
              <p class="field-note">
                Rules run top to bottom before the lists above, and the first
                match decides. Use them for scheduled site exceptions, per-user
                exceptions, or blocking part of a site.
              </p>

              {#each draft.rules as rule, index (index)}
                {#if index !== bedtimeIndex}
                  <div class="rule">
                    <div class="rule-head">
                      <h5>Rule {index + 1}</h5>
                      <div class="rule-head-controls">
                        <Toggle
                          size="sm"
                          labelText="Enabled"
                          hideLabel
                          bind:toggled={rule.enabled}
                        />
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
                        <Button
                          size="small"
                          kind="danger-ghost"
                          on:click={() => removeRule(index)}>Remove rule</Button
                        >
                      </div>
                    </div>

                    <Row>
                      <Column sm={4} md={4} lg={5}
                        ><TextInput
                          labelText="Name"
                          placeholder="After school"
                          bind:value={rule.name}
                        /></Column
                      >
                      <Column sm={4} md={4} lg={5}>
                        <Select
                          labelText="Action"
                          bind:selected={rule.action}
                          on:change={touchRules}
                        >
                          <SelectItem value="block" text="Block" />
                          <SelectItem value="allow" text="Allow" />
                        </Select>
                      </Column>
                      <Column sm={4} md={8} lg={6}>
                        <Select
                          labelText="Applies to"
                          selected={rule.target.all_traffic ? "all" : "listed"}
                          on:change={(event) => setTargetKind(rule, event)}
                        >
                          <SelectItem
                            value="listed"
                            text="Selected categories and domains"
                          />
                          <SelectItem value="all" text="All traffic" />
                        </Select>
                      </Column>
                    </Row>

                    {#if !rule.target.all_traffic}
                      <div class="rule-target">
                        <Categoryselect
                          {categories}
                          compact
                          selection={rule.target.categories}
                          on:change={(event) => {
                            rule.target.categories = event.detail;
                            touchRules();
                          }}
                        />
                        <Domainlist
                          label="Domain"
                          entries={rule.target.domains}
                          on:change={(event) => {
                            rule.target.domains = event.detail;
                            touchRules();
                          }}
                        />
                      </div>
                    {/if}

                    <Row>
                      <Column sm={4} md={4} lg={8}>
                        <Select
                          labelText="When"
                          selected={scheduleChoice(rule)}
                          helperText={rule.schedule
                            ? scheduleLabel(rule.schedule) +
                              " (" +
                              (rule.schedule.timezone || "UTC") +
                              ")"
                            : ""}
                          on:change={(event) => onScheduleChange(rule, event)}
                        >
                          <SelectItem value="" text="Always" />
                          {#each schedulePresets as preset (preset.id)}<SelectItem
                              value={preset.id}
                              text={presetLabel(preset)}
                            />{/each}
                          <SelectItem
                            value={SCHEDULE_PRESET_CUSTOM}
                            text="Custom hours"
                          />
                        </Select>
                        {#if rule.schedule && !rule.schedule.preset}
                          <div class="window-row">
                            <TimePicker
                              size="sm"
                              labelText="From"
                              placeholder="HH:MM"
                              pattern={TIME_PATTERN}
                              value={rule.schedule.windows[0]?.from || ""}
                              on:change={(event) =>
                                onWindowTimeChange(rule, "from", event)}
                            />
                            <TimePicker
                              size="sm"
                              labelText="To"
                              placeholder="HH:MM"
                              pattern={TIME_PATTERN}
                              value={rule.schedule.windows[0]?.to || ""}
                              on:change={(event) =>
                                onWindowTimeChange(rule, "to", event)}
                            />
                          </div>
                          <MultiSelect
                            titleText="Days"
                            label="Every day"
                            items={WEEKDAYS}
                            selectedIds={rule.schedule.weekdays.map(String)}
                            on:select={(event) =>
                              setWeekdays(rule, event.detail.selectedIds)}
                          />
                        {/if}
                      </Column>
                      <Column sm={4} md={4} lg={8}>
                        {#if users.length}
                          <MultiSelect
                            titleText="Only for proxy users"
                            label="Everyone on this policy"
                            items={users.map((user) => ({
                              id: user,
                              text: user,
                            }))}
                            selectedIds={rule.users}
                            on:select={(event) =>
                              setRuleUsers(rule, event.detail.selectedIds)}
                          />
                        {/if}
                      </Column>
                    </Row>

                    {#if conditionsAllowed(rule)}
                      <div class="rule-conditions">
                        <Accordion>
                          <AccordionItem
                            title="Block only part of the site"
                            open={rule.url_regexes.length > 0 ||
                              rule.blocked_content_types.length > 0}
                          >
                            <Row>
                              <Column sm={4} md={4} lg={8}>
                                <div class="list-entry">
                                  <TextInput
                                    labelText="URL pattern (regular expression)"
                                    placeholder="/shorts/"
                                    bind:value={rule.url_draft}
                                    on:keydown={(event) =>
                                      event.key === "Enter" &&
                                      addListEntry(
                                        rule,
                                        "url_regexes",
                                        "url_draft",
                                      )}
                                  />
                                  <Button
                                    size="small"
                                    kind="tertiary"
                                    on:click={() =>
                                      addListEntry(
                                        rule,
                                        "url_regexes",
                                        "url_draft",
                                      )}>Add pattern</Button
                                  >
                                </div>
                                {#if rule.url_regexes.length}<div class="tags">
                                    {#each rule.url_regexes as pattern (pattern)}<Tag
                                        filter
                                        size="sm"
                                        on:close={() =>
                                          removeListEntry(
                                            rule,
                                            "url_regexes",
                                            pattern,
                                          )}>{pattern}</Tag
                                      >{/each}
                                  </div>{/if}
                              </Column>
                              <Column sm={4} md={4} lg={8}>
                                <div class="list-entry">
                                  <TextInput
                                    labelText="Response type"
                                    placeholder="video/"
                                    bind:value={rule.content_type_draft}
                                    on:keydown={(event) =>
                                      event.key === "Enter" &&
                                      addListEntry(
                                        rule,
                                        "blocked_content_types",
                                        "content_type_draft",
                                        true,
                                      )}
                                  />
                                  <Button
                                    size="small"
                                    kind="tertiary"
                                    on:click={() =>
                                      addListEntry(
                                        rule,
                                        "blocked_content_types",
                                        "content_type_draft",
                                        true,
                                      )}>Add type</Button
                                  >
                                </div>
                                {#if rule.blocked_content_types.length}<div
                                    class="tags"
                                  >
                                    {#each rule.blocked_content_types as contentType (contentType)}<Tag
                                        filter
                                        size="sm"
                                        on:close={() =>
                                          removeListEntry(
                                            rule,
                                            "blocked_content_types",
                                            contentType,
                                          )}>{contentType}</Tag
                                      >{/each}
                                  </div>{/if}
                              </Column>
                            </Row>
                          </AccordionItem>
                        </Accordion>
                      </div>
                    {/if}

                    {#if needsProxy(rule)}
                      <p class="field-note proxy-note">
                        DNS cannot see URLs, response types, or proxy logins, so
                        this rule is enforced by the proxy only. Devices that
                        use GateSentry for DNS alone are not affected by it.
                      </p>
                    {/if}
                  </div>
                {/if}
              {/each}

              <Button size="small" kind="tertiary" on:click={addRule}
                >Add rule</Button
              >
            </fieldset>
          </div>
        </AccordionItem>
      </Accordion>
    </FormGroup>
  </div>

  <div class="group-actions" role="group" aria-label="Policy actions">
    <Button size="small" type="submit" disabled={saving}>Save policy</Button>
    <Button size="small" type="button" kind="ghost" on:click={onCancel}
      >Cancel</Button
    >
  </div>
</form>

<style>
  /* Carbon v10's compiled g10 theme exposes literal colors rather than --cds-*
     custom properties: #161616 text-01, #525252 text-02, and #f4f4f4 for the
     nested rule surface under a white tile. */
  h5 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.375rem;
  }
  .policy-form {
    padding-bottom: 4rem;
  }
  .form-section {
    margin-top: 1.5rem;
    padding: 1.5rem;
    border: 1px solid #e0e0e0;
  }
  .form-section :global(.bx--label) {
    font-size: 1rem;
    font-weight: 600;
    line-height: 1.375rem;
  }
  .section-description {
    max-width: 46rem;
    margin: 0 0 1rem;
    color: #525252;
    font-size: 0.875rem;
    line-height: 1.25rem;
  }
  .advanced-content {
    padding: 0 1rem 1rem;
  }
  .first-field {
    margin-top: 0;
  }
  .name-field {
    max-width: 24rem;
  }
  .bedtime-row {
    flex-wrap: wrap;
    align-items: flex-end;
    margin-top: 0.75rem;
    margin-bottom: 0;
  }
  .advanced-form-section :global(.bx--accordion) {
    margin: 0 -1.5rem -1.5rem;
  }
  .rule-conditions {
    margin-top: 0.75rem;
  }
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
    margin: 0.25rem 0 0.75rem;
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
  .rule-target {
    margin: 0.5rem 0 1rem;
  }
  .proxy-note {
    margin-top: 0.75rem;
    margin-bottom: 0;
  }
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
    position: sticky;
    z-index: 2;
    bottom: 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    justify-content: flex-end;
    margin: 1.5rem -1rem -1rem;
    padding: 0.75rem 1rem;
    border-top: 1px solid #e0e0e0;
    background: rgba(255, 255, 255, 0.96);
  }
  @media (max-width: 48rem) {
    .form-section {
      padding: 1rem;
    }
    .advanced-form-section :global(.bx--accordion) {
      margin: 0 -1rem -1rem;
    }
    .rule-head {
      align-items: flex-start;
      flex-direction: column;
    }
    .rule-head-controls {
      flex-wrap: wrap;
    }
    .window-row,
    .list-entry {
      align-items: stretch;
      flex-direction: column;
    }
    .list-entry :global(.bx--btn) {
      width: 100%;
      max-width: none;
    }
  }
  @media (max-width: 30rem) {
    .group-actions :global(.bx--btn) {
      flex: 1;
    }
  }
</style>
