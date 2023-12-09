<script lang="ts">
  import { Button, ButtonSet, Column, Row } from "carbon-components-svelte";
  import Ruleform from "./rform.svelte";
  import { Add, Save } from "carbon-icons-svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";

  let rules = [];
  let apiRules = null;
  let isExpanded = {};

  function addRule() {
    console.log("add rule");
    rules = [
      ...rules,
      {
        timeRestriction: { action: "block" },
      },
    ];
  }

  let updateNetwork = null;

  function removeRule(e) {
    const index = e.detail;
    rules = rules.filter((_, i) => i !== index);
  }

  const saveRules = async () => {
    console.log("Rules = ", rules);
    const json = JSON.stringify(rules, null, 2);

    await updateNetwork(json);
  };

  function toggleRule(index) {
    isExpanded[index] = !isExpanded[index];
  }

  $: {
    if (apiRules !== null) {
      rules = JSON.parse(apiRules);
    }
  }
</script>

<ConnectedSettingInput
  keyName="rules"
  type="external"
  disableOnblur={true}
  bind:data={apiRules}
  bind:updateNetwork
/>
<Row>
  <Column>
    {#if apiRules !== null}{:else}{/if}

    {#each rules as rule, index (index)}
      <Ruleform
        {rule}
        {index}
        on:remove={removeRule}
        {toggleRule}
        isOpen={isExpanded[index] ?? false}
      />
    {/each}
  </Column>
</Row>
<Row>
  <Column>
    <div style="display: flex">
      <div style="flex-grow: 1"></div>
      <div style="padding-right:20px;">
        <div style="position: relative; left:-6em;">
          <ButtonSet>
            <Button on:click={saveRules} kind="primary" icon={Save}
              >Save Rules</Button
            >&nbsp;
            <Button on:click={addRule} icon={Add}>Add Rule</Button>
          </ButtonSet>
        </div>
      </div>
    </div>
  </Column>
</Row>
