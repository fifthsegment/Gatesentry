<script>
  import {
    SelectItem,
    TimePicker,
    TimePickerSelect,
  } from "carbon-components-svelte";

  export let value = "";
  export let label = "";
</script>

<TimePicker labelText={label} placeholder="hh:mm" bind:value></TimePicker>
