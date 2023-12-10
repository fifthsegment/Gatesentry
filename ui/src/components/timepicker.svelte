<script>
  import {
    Select,
    SelectItem,
    TimePicker,
    TimePickerSelect,
  } from "carbon-components-svelte";

  export let value = "";
  export let label = "";

  // Define an array of all hours in the day
  const hours = Array.from({ length: 24 }, (_, i) => ({
    id: i,
    text: `${i.toString().padStart(2, "0")}:00`,
  }));
  // let selectedHour = hours[0];
  // value = hours[0].id.toString().padStart(2, "0");
  const onChange = (event) => {
    const found = hours.find((hour) => hour.id === Number(event.target.value));
    value = found.id.toString().padStart(2, "0");
  };
</script>

<!-- <TimePicker labelText={label} placeholder="hh:mm" bind:value></TimePicker> -->

<Select labelText={label} on:change={onChange}>
  <SelectItem
    disabled
    hidden
    value="placeholder-item"
    text="Choose an option"
  />
  {#each hours as hour}
    <SelectItem value={hour.id} text={hour.text} />
  {/each}
</Select>
