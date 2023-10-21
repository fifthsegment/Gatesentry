<script lang="ts">
  import { Button, ButtonSet, Column, Row } from "carbon-components-svelte";
  import Ruleform from "./rform.svelte";
  import { Add, Save } from "carbon-icons-svelte";

  let rules = [];

  function addRule() {
    console.log("add rule");
    rules = [
      ...rules,
      {
        timeRestriction: { action: "block" },
      },
    ];
  }

  function removeRule(e) {
    const index = e.detail;
    console.log("Removing rule", index);
    rules = rules.filter((_, i) => i !== index);
  }

  function saveRules() {
    const json = JSON.stringify({ rules }, null, 2);
    console.log(json); // Replace with actual save logic
  }
</script>

<Row>
  <Column>
    {#each rules as rule, index (index)}
      <Ruleform {rule} {index} on:remove={removeRule} />
    {/each}
    <div style="padding-right:20px;">
      <div style=" ">
        <ButtonSet>
          <Button on:click={saveRules} kind="secondary" icon={Save}
            >Save Rules</Button
          >
          <Button on:click={addRule} icon={Add}>Add Rule</Button>
        </ButtonSet>
      </div>
    </div>
  </Column>
</Row>
