<script lang="ts">
  import { Button, ButtonSet, Column, Row } from "carbon-components-svelte";
  import Ruleform from "./rform.svelte";
  import { Add, Save } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { onMount } from "svelte";
  import { notificationstore } from "../../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../../lib/utils";
  import { _ } from "svelte-i18n";

  let rules = [];
  let editingRuleIndex = -1;

  const API_SETTING_KEY = "rules";

  const loadRulesAPI = async () => {
    try {
      const json = await $store.api.getSetting(API_SETTING_KEY);
      const parsedJson = JSON.parse(json.Value, null);
      rules = [...parsedJson];
    } catch (error) {
      console.error(
        "[GatesentryUI:loadRulesAPI] Unable to load settings (possibly due to logout)",
      );
      notificationstore.add(
        createNotificationError(
          { subtitle: $_("Unable to load rules from the API") },
          $_,
        ),
      );
    }
  };

  const updateNetwork = async (updateData) => {
    const response = await $store.api.setSetting(
      API_SETTING_KEY,
      JSON.stringify(updateData),
    );
    if (response === false) {
      notificationstore.add(
        createNotificationError({ subtitle: $_("Unable to save setting") }, $_),
      );
    } else {
      notificationstore.add(
        createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
      );
    }
    await loadRulesAPI();
  };

  function addRule() {
    rules = [
      ...rules,
      {
        timeRestriction: { from: "", to: "" },
        action: "block",
        contentSize: 0,
      },
    ];
    editingRuleIndex = rules.length - 1;
  }

  function removeRule(e) {
    const index = e.detail;
    rules = rules.filter((_, i) => i !== index);
    editingRuleIndex = -1;
    saveRules();
  }

  function saveRules() {
    // const json = JSON.stringify(rules);
    updateNetwork(rules);
  }

  onMount(async () => {
    await loadRulesAPI();
  });
</script>

<Row>
  <Column>
    <div style="padding-right:20px;">
      <div style="display: flex;">
        <span style="margin-left: auto; position: relative; left:1em;">
          <!-- <Button
            size="small"
            on:click={saveRules}
            icon={Save}
            kind="secondary"
          /> -->
          <Button size="small" on:click={addRule} icon={Add} />
        </span>
      </div>
    </div>
    {#each rules as rule, index (index)}
      <Ruleform
        {rule}
        {index}
        on:cancel={() => (editingRuleIndex = -1)}
        on:remove={removeRule}
        on:save={(opts) => {
          const rule = opts.detail.rule;
          const index = opts.detail.index;
          rules[index] = rule;
          editingRuleIndex = -1;
          console.log("Rule = ", rule);
          saveRules();
        }}
        isEditing={editingRuleIndex == index}
        on:edit={() => (editingRuleIndex = index)}
      />
    {/each}
  </Column>
</Row>
