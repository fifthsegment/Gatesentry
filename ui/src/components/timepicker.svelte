<script>
  import {
    SelectItem,
    TimePicker,
    TimePickerSelect,
  } from "carbon-components-svelte";

  export let value = "";
  export let label = "";
  let timeValidationMessage = "";

  function validateTime(event) {
    const pattern = /^([01]?[0-9]|2[0-3]):[0-5][0-9]$/;
    if (!pattern.test(event.target.value)) {
      timeValidationMessage = "Please enter a valid time in HH:MM format.";
    } else {
      timeValidationMessage = "";
    }
  }
</script>

<TimePicker
  labelText={label}
  placeholder="hh:mm"
  bind:value
  on:input={validateTime}
  type="text"
></TimePicker>
<div class="error-message">{timeValidationMessage}</div>

<style>
  .error-message {
    color: red;
  }
</style>
