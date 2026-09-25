<script lang="ts">
  import { InlineLoading, TextInput } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";

  import { gsNavigate } from "../lib/navigate";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";

  export let keyName: string;
  export let labelText: string;
  export let title: string;
  export let helperText: string = "";
  export let type: string = "text";
  export let disabled = false;
  export let disableOnblur = false;

  const SETTING_GENERAL_SETTINGS = "general_settings";

  let data: Record<string, string> | null = null;
  let internalFormValue: Record<string, string> | null = null;
  let loading = true;
  let saving = false;

  export const updateDataOnBackend = async () => {
    if (internalFormValue) {
      await updateNetwork(internalFormValue);
    }
  };

  const notifyLoadError = () => {
    notificationstore.add(
      createNotificationError({ subtitle: $_("Unable to load settings") }, $_),
    );
  };

  const loadAPIData = async () => {
    loading = true;
    try {
      const json = await $store.api.getSetting(SETTING_GENERAL_SETTINGS);
      data = JSON.parse(json.Value);
    } catch (error) {
      console.error(
        "[GatesentryUI] Unable to load settings (possibly due to logout)",
      );
      notifyLoadError();
    } finally {
      loading = false;
    }
  };

  const updateNetwork = async (updateData: Record<string, string>) => {
    saving = true;
    try {
      const response = await $store.api.setSetting(
        SETTING_GENERAL_SETTINGS,
        JSON.stringify(updateData),
      );
      if (response === false) {
        notificationstore.add(
          createNotificationError(
            { subtitle: $_("Unable to save setting") },
            $_,
          ),
        );
        return;
      }

      notificationstore.add(
        createNotificationSuccess({ subtitle: $_("Setting updated") }, $_),
      );
      internalFormValue = null;
      if (keyName === "admin_password" || keyName === "admin_username") {
        store.logout();
        gsNavigate("/login");
        return;
      }
      await loadAPIData();
    } catch (error) {
      notificationstore.add(
        createNotificationError({ subtitle: $_("Unable to save setting") }, $_),
      );
    } finally {
      saving = false;
    }
  };

  const updateField = async (event: FocusEvent) => {
    if (!data) return;
    const value = (event.currentTarget as HTMLInputElement).value;
    if (value === data[keyName]) return;

    internalFormValue = {
      ...data,
      [keyName]: value,
    };
    if (!disableOnblur) {
      await updateNetwork(internalFormValue);
    }
  };

  onMount(loadAPIData);
</script>

<div class="connected-setting" aria-busy={loading || saving}>
  <TextInput
    {title}
    {labelText}
    {helperText}
    {type}
    disabled={disabled || loading || saving}
    value={(data && data[keyName]) ?? ""}
    on:blur={updateField}
  />
  {#if loading}
    <InlineLoading description={$_("Loading setting…")} />
  {:else if saving}
    <InlineLoading description={$_("Saving setting…")} />
  {/if}
</div>

<style>
  .connected-setting {
    min-width: 0;
  }

  .connected-setting :global(.bx--inline-loading) {
    margin-top: 0.5rem;
  }
</style>
