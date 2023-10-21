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
  export let rule = {
    domain: "",
    timeRestriction: { from: "", to: "", action: "block" },
    user: "",
    dataLimitPer5Seconds: 0,
  };

  export let index;
  import { createEventDispatcher } from "svelte";
  import Timepicker from "../../components/timepicker.svelte";
  import { RowDelete } from "carbon-icons-svelte";
  const dispatch = createEventDispatcher();
</script>

<div class="simple-border">
  <Row>
    <Column sm={4} md={4} lg={4}>
      <h5>Rule {index + 1}</h5>
    </Column>
    <Column sm={12} md={12} lg={12}>
      <TextInput
        type="text"
        bind:value={rule.domain}
        placeholder="Domain"
        style="margin-bottom: 10px;"
        labelText="Domain"
      />
      <br />
      <div style="display: flex;">
        <span
          ><Timepicker
            bind:value={rule.timeRestriction.from}
            label="From"
          /></span
        >
        <span style="margin: 0 10px;"></span>
        <span
          ><Timepicker bind:value={rule.timeRestriction.to} label="To" /></span
        >
        <span style="margin: 0 10px;"></span>
        <Dropdown
          titleText="Action"
          selectedId={rule.timeRestriction.action}
          on:select={(e) => {
            // @ts-ignore
            rule.timeRestriction.action = e.detail.selectedId;
          }}
          items={[
            { id: "block", text: "Block" },
            { id: "allow", text: "Allow" },
          ]}
        />
      </div>

      <br />
      <TextInput
        type="text"
        bind:value={rule.user}
        placeholder="User"
        labelText="User"
      />
      <br />
      <span>
        <TextInput
          type="number"
          bind:value={rule.dataLimitPer5Seconds}
          placeholder="Data Limit per 5 seconds"
          style="margin-bottom: 10px;"
          labelText="Data limit"
        />Megabytes / second
      </span>
    </Column>
  </Row>
  <div style="display: flex; justify-content: flex-end;">
    <Button icon={RowDelete} on:click={() => dispatch("remove", index)}
      >Remove</Button
    >
  </div>
</div>
