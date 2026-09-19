<script lang="ts">
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { onMount } from "svelte";
  import { Button, InlineNotification, Tag } from "carbon-components-svelte";

  let status: any = null;
  let loading = false;
  let error = "";

  async function loadStatus() {
    loading = true;
    error = "";
    try {
      status = await $store.api.doCall("/certificate/inspection");
    } catch (e) {
      error = e instanceof Error ? e.message : "Failed to load inspection status";
    } finally {
      loading = false;
    }
  }

  onMount(loadStatus);
</script>

<section class="inspection gs-card">
  <h3>{$_("HTTPS Inspection Status")}</h3>

  {#if error}
    <InlineNotification kind="error" title={$_("Error")} subtitle={error} on:close={() => (error = "")} />
  {/if}

  {#if loading && !status}
    <p>{$_("Loading…")}</p>
  {/if}

  {#if status}
    <div class="status-row">
      <strong>{$_("Enabled")}:</strong>
      <Tag type={status.enabled ? "green" : "gray"} size="sm">
        {status.enabled ? $_("Enabled") : $_("Not enabled")}
      </Tag>
    </div>

    {#if status.certificate}
      <div class="cert-info">
        <p><strong>{$_("Certificate subject")}:</strong> {status.certificate.name}</p>
        <p><strong>{$_("Not before")}:</strong> {status.certificate.not_before}</p>
        <p><strong>{$_("Not after")}:</strong> {status.certificate.not_after}</p>
        {#if status.certificate.expired}
          <InlineNotification kind="error" title={$_("Expired")} subtitle={$_("The gateway CA certificate has expired. Renew it and reinstall it on clients.")} />
        {/if}
      </div>
    {/if}

    {#if status.certificate_error}
      <InlineNotification kind="warning" title={$_("Certificate error")} subtitle={status.certificate_error} />
    {/if}

    <div class="exclusions">
      <p><strong>{$_("Exclusion hosts (tunnelled without inspection)")}:</strong></p>
      <p class="muted">{status.exclusion_note}</p>
      {#if status.exclusions && status.exclusions.length > 0}
        <ul>
          {#each status.exclusions as host}
            <li>{host}</li>
          {/each}
        </ul>
      {:else}
        <p class="muted">{$_("No exclusions configured")}</p>
      {/if}
    </div>

    <div class="counts">
      <p><strong>{$_("Inspect count")}:</strong> {status.inspect_count}</p>
      <p><strong>{$_("Bypass count")}:</strong> {status.bypass_count}</p>
      <p class="muted window">{$_("Window")}: {status.window}</p>
    </div>

    <p class="caveat muted">{status.coverage_caveat}</p>
  {/if}

  <Button kind="tertiary" size="small" disabled={loading} on:click={loadStatus}>
    {loading ? $_("Refreshing…") : $_("Refresh")}
  </Button>
</section>

<style>
  .inspection { margin-top: 2rem; padding: 1.5rem; }
  .status-row { display: flex; align-items: center; gap: .5rem; margin-bottom: 1rem; }
  .cert-info { margin-bottom: 1rem; }
  .cert-info p { margin: .25rem 0; }
  .exclusions { margin-bottom: 1rem; }
  .exclusions ul { margin: .5rem 0; padding-left: 1.5rem; }
  .counts { margin-bottom: 1rem; }
  .counts p { margin: .25rem 0; }
  .caveat { font-style: italic; }
  .muted { color: var(--cds-text-secondary, #525252); font-size: 0.875rem; }
  .window { font-size: 0.8rem; }
  .inspection :global(.bx--btn) { margin-top: 1rem; }
</style>
