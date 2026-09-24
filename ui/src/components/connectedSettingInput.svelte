<script lang="ts">
  import {
    InlineLoading,
    InlineNotification,
    RadioButton,
    RadioButtonGroup,
    TextInput,
  } from "carbon-components-svelte";
  import { onDestroy, onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import { createNotificationError, createNotificationSuccess } from "../lib/utils";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";

  type RadioOption = {
    value: string;
    label: string;
    disabled?: boolean;
  };

  export let keyName: string;
  export let labelText: string;
  export let title: string;
  export let helperText: string = "";
  export let type: string;
  export let disabled = false;
  export let disableOnblur = false;
  export let radioOptions: RadioOption[] = [];
  export let orientation: "horizontal" | "vertical" = "horizontal";
  export let onSaved: (value: string) => void | Promise<void> = () => {};

  let data = "";
  let persistedValue = "";
  let internalFormValue = "";
  let loaded = false;
  let saving = false;
  let saveError = "";

  export const updateDataOnBackend = async () => {
    return updateNetwork(internalFormValue);
  };

  const loadAPIData = async () => {
    try {
      const json = await $store.api.getSetting(keyName);
      const value = json?.Value ?? json?.value ?? "";
      data = String(value);
      persistedValue = data;
      internalFormValue = data;
    } catch (error) {
      saveError = $_("Unable to load setting");
      console.error(
        "[GatesentryUI] Unable to load settings (possibly due to logout)",
      );
    }
  };

  const updateNetwork = async (keyValue: string) => {
    if (saving || keyValue === persistedValue) return true;

    saving = true;
    saveError = "";
    try {
      const response = await $store.api.setSetting(keyName, keyValue);
      if (response === false) {
        throw new Error($_("Unable to save setting"));
      }

      persistedValue = keyValue;
      data = keyValue;
      notificationstore.add(
        createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
      );
      await onSaved(keyValue);
      return true;
    } catch (error) {
      data = persistedValue;
      internalFormValue = persistedValue;
      saveError =
        error instanceof Error && error.message
          ? error.message
          : $_("Unable to save setting");
      notificationstore.add(
        createNotificationError({ subtitle: saveError }, $_),
      );
      return false;
    } finally {
      saving = false;
    }
  };

  const updateInternalValue = (event: Event) => {
    internalFormValue = (event.currentTarget as HTMLInputElement).value;
  };

  const updateField = async (event: Event) => {
    const value = (event.currentTarget as HTMLInputElement).value;
    internalFormValue = value;
    if (value === persistedValue || disableOnblur) return;
    await updateNetwork(value);
  };

  const updateFieldRadio = async (event: CustomEvent<string>) => {
    const value = String(event.detail ?? data ?? "");
    internalFormValue = value;
    if (value === persistedValue || disableOnblur) return;
    await updateNetwork(value);
  };

  onMount(async () => {
    await loadAPIData();
    loaded = true;
  });

  onDestroy(() => {
    data = "";
    persistedValue = "";
    internalFormValue = "";
    loaded = false;
  });
</script>

<div class="connected-setting">
  {#if loaded}
    {#if type === "radio"}
      <RadioButtonGroup
        legendText={labelText}
        {orientation}
        disabled={disabled || saving}
        bind:selected={data}
        on:change={updateFieldRadio}
      >
        {#if radioOptions.length > 0}
          {#each radioOptions as option (option.value)}
            <RadioButton
              id={`${keyName}-${option.value}`}
              value={option.value}
              labelText={option.label}
              disabled={option.disabled ?? false}
            />
          {/each}
        {:else}
          <RadioButton
            id={`${keyName}-true`}
            value="true"
            labelText={$_("True")}
          />
          <RadioButton
            id={`${keyName}-false`}
            value="false"
            labelText={$_("False")}
          />
        {/if}
      </RadioButtonGroup>
      {#if helperText}
        <p class="helper-text">{helperText}</p>
      {/if}
    {:else}
      <TextInput
        {title}
        {labelText}
        {helperText}
        {type}
        {disabled}
        value={data}
        on:input={updateInternalValue}
        on:blur={updateField}
      />
    {/if}
  {/if}

  {#if saving}
    <InlineLoading description={$_("Saving…")} />
  {/if}
  {#if saveError}
    <InlineNotification
      lowContrast
      kind="error"
      title={$_("Setting not saved")}
      subtitle={saveError}
      on:close={() => (saveError = "")}
    />
  {/if}
</div>

<style>
  .connected-setting {
    display: grid;
    gap: 0.5rem;
  }

  .helper-text {
    margin: -0.25rem 0 0;
    color: var(--cds-text-helper, #6f6f6f);
    font-size: 0.75rem;
    line-height: 1.34;
  }
</style>
