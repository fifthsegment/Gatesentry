<script lang="ts">
  import {
    Button,
    Column,
    Dropdown,
    Row,
    SelectItem,
    TextInput,
    TimePicker,
    TimePickerSelect,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";

  export let rule = {
    domain: "",
    timeRestriction: { from: "", to: "", action: "block" },
    user: "",
    contentSize: 0,
    contentType: "",
  };

  export let index;
  export let isOpen = false;

  export let toggleRule = (index) => {
    isOpen = !isOpen;
  };
  import { createEventDispatcher } from "svelte";
  import Timepicker from "../../components/timepicker.svelte";
  import { RowDelete } from "carbon-icons-svelte";
  const dispatch = createEventDispatcher();
</script>

{#if isOpen === false}
  <div
    class="simple-border rule"
    style="cursor:pointer"
    on:click={() => toggleRule(index)}
  >
    [+] Rule {index + 1}
  </div>
{:else if isOpen}
  <div class="simple-border rule">
    <Row>
      <Column sm={4} md={4} lg={4}>
        <h5 on:click={() => toggleRule(index)} style="cursor:pointer">
          [-] Rule {index + 1}
        </h5>
      </Column>
      <Column sm={12} md={12} lg={12}>
        <div class="rule-line">
          <span class="rule-description"> Action </span>
          <span class="rule-field"
            ><Dropdown
              titleText="Action"
              selectedId={rule.timeRestriction.action}
              on:select={(e) => {
                // @ts-ignore
                rule.timeRestriction.action = e.detail.selectedId;
              }}
              items={[
                { id: "block", text: "Block" },
                { id: "allow", text: "Allow" },
                { id: "sendTo", text: "Send data to service" },
              ]}
            /></span
          >
          <span class="rule-comment">
            {$_("Will ")} <strong>{rule.timeRestriction.action}</strong>
            {$_("traffic matching the following conditions.... ")}
          </span>
        </div>
        <div class="rule-line">
          <span class="rule-description"> {$_("Domain = ")} </span>
          <span class="rule-field"
            ><TextInput
              type="text"
              bind:value={rule.domain}
              placeholder="Domain"
              style="margin-bottom: 10px;"
              labelText="Domain"
            />
          </span>

          {#if rule.domain != ""}
            <span class="rule-comment">
              {$_("... domain is ")} <strong>{rule.domain}</strong>
            </span>
          {/if}
        </div>
        <div class="rule-line">
          <span class="rule-description"> {$_("Time = ")} </span>
          <span class="rule-field" style="display:flex">
            <span
              ><Timepicker
                bind:value={rule.timeRestriction.from}
                label="From"
              /></span
            >
            <span style="margin: 0 10px;"></span>
            <span
              ><Timepicker
                bind:value={rule.timeRestriction.to}
                label="To"
              /></span
            >
          </span>

          {#if rule.timeRestriction.from != "" && rule.timeRestriction.to != ""}
            <span class="rule-comment">
              {$_("...time is between ")}
              <strong>{rule.timeRestriction.from}</strong>
              {$_("and")} <strong>{rule.timeRestriction.to}</strong>
            </span>
          {/if}
        </div>
        <div class="rule-line">
          <span class="rule-description"> {$_("User = ")} </span>
          <span class="rule-field"
            ><TextInput
              type="text"
              bind:value={rule.user}
              placeholder="User"
              style="margin-bottom: 10px;"
              labelText="User"
            />
          </span>

          {#if rule.user != ""}
            <span class="rule-comment">
              {$_("...user is ")} <strong>{rule.user}</strong>
            </span>
          {/if}
        </div>
        <div class="rule-line">
          <span class="rule-description"> {$_("Content Size = ")} </span>

          <span class="rule-field">
            <TextInput
              type="number"
              bind:value={rule.contentSize}
              placeholder="Content Size (MB)"
              style="margin-bottom: 10px;"
              labelText="Content size (MB)"
            />
          </span>

          {#if rule.contentSize !== 0 && rule.contentSize !== null}
            <span class="rule-comment">
              {$_("...content size is greater than ")}
              <strong>{rule.contentSize}</strong>
              {$_("MB")}
            </span>
          {/if}
        </div>

        <div class="rule-line">
          <span class="rule-description"> {$_("Content Type = ")} </span>

          <span class="rule-field">
            <TextInput
              type="text"
              bind:value={rule.contentType}
              placeholder="Content Type"
              style="margin-bottom: 10px;"
              labelText="Content Type"
            />
          </span>

          {#if rule.contentType != ""}
            <span class="rule-comment">
              {$_("...content type is ")}
              <strong>{rule.contentType}</strong>
            </span>
          {/if}
        </div>
      </Column>
    </Row>
    <div style="display: flex; justify-content: flex-end;">
      <Button
        icon={RowDelete}
        kind="danger"
        on:click={() => dispatch("remove", index)}>Remove</Button
      >
    </div>
  </div>
{/if}

<style>
  .rule {
    margin-bottom: 1em;
  }

  .rule-line {
    display: flex;
    margin-bottom: 10px;
    align-items: center;
  }

  .rule-description {
    width: 100px;
  }

  .rule-field {
    width: 250px;
  }

  .rule-comment {
    flex: 1;
    font-size: 1em;
    color: gray;
    background-color: aliceblue;
    display: inline-block;
    height: 60px;
    margin-left: 10px;
    padding-top: 1em;
    padding-left: 1em;
  }
</style>
