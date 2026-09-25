<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Button,
    InlineNotification,
  } from "carbon-components-svelte";
  import { Renew } from "carbon-icons-svelte";
  import { onDestroy, onMount } from "svelte";
  import { _ } from "svelte-i18n";

  import DnsArecords from "../../components/dnsArecords.svelte";
  import Dnslists from "../../components/dnslists.svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
  import PageShell from "../../components/layout/PageShell.svelte";
  import ResourceState from "../../components/layout/ResourceState.svelte";
  import SectionPanel from "../../components/layout/SectionPanel.svelte";
  import { store } from "../../store/apistore";

  type DnsInfo = {
    number_domains_blocked?: number;
    last_updated?: number;
    next_update?: number;
  };

  let dnsInfo: DnsInfo | null = null;
  let loading = true;
  let refreshing = false;
  let statusError = "";
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;
  let destroyed = false;

  const loadDnsInfo = async () => {
    if (refreshing || destroyed) return;
    refreshing = true;
    statusError = "";
    try {
      dnsInfo = await $store.api.doCall("/dns/info");
    } catch (caught) {
      statusError =
        caught instanceof Error && caught.message
          ? caught.message
          : $_("DNS status could not be loaded.");
    } finally {
      loading = false;
      refreshing = false;
    }
  };

  const getHumanTime = (time?: number) => {
    if (!time) return $_("Not scheduled");
    const date = new Date(time * 1000);
    return Number.isNaN(date.getTime()) ? $_("Unknown") : date.toLocaleString();
  };

  const onUpdateDnsInfo = () => {
    dnsInfo = null;
    loading = true;
    if (refreshTimer) clearTimeout(refreshTimer);
    refreshTimer = setTimeout(() => {
      refreshTimer = null;
      loadDnsInfo();
    }, 3000);
  };

  onMount(loadDnsInfo);
  onDestroy(() => {
    destroyed = true;
    if (refreshTimer) clearTimeout(refreshTimer);
  });
</script>

<PageShell
  title={$_("DNS server")}
  description={$_(
    "Review DNS block-list health, choose an upstream resolver, and manage local names and list sources.",
  )}
>
  <svelte:fragment slot="breadcrumb">
    <Breadcrumb noTrailingSlash>
      <BreadcrumbItem href="/">{$_("Dashboard")}</BreadcrumbItem>
      <BreadcrumbItem>{$_("DNS server")}</BreadcrumbItem>
    </Breadcrumb>
  </svelte:fragment>
  <svelte:fragment slot="actions">
    <Button
      kind="ghost"
      size="small"
      icon={Renew}
      disabled={refreshing}
      on:click={loadDnsInfo}
    >
      {refreshing ? $_("Refreshing…") : $_("Refresh status")}
    </Button>
  </svelte:fragment>

  {#if statusError}
    <InlineNotification
      kind="error"
      title={$_("DNS status unavailable")}
      subtitle={statusError}
      on:close={() => (statusError = "")}
    />
  {/if}

  <div class="dns-grid">
    <div class="dns-column">
      <SectionPanel
        title={$_("DNS status")}
        description={$_("Block-list totals and update schedule reported by the running DNS service.")}
      >
        {#if loading && !dnsInfo}
          <ResourceState state="loading" message={$_("Loading DNS status…")} />
        {:else if !dnsInfo}
          <ResourceState
            state="error"
            title={$_("DNS status is unavailable")}
            message={statusError || $_("Try refreshing after the DNS service is ready.")}
            retryLabel={$_("Retry")}
            onRetry={loadDnsInfo}
          />
        {:else}
          <dl class="status-list">
            <div>
              <dt>{$_("Blocked domains")}</dt>
              <dd>{dnsInfo.number_domains_blocked ?? 0}</dd>
            </div>
            <div>
              <dt>{$_("Last updated")}</dt>
              <dd>{getHumanTime(dnsInfo.last_updated)}</dd>
            </div>
            <div>
              <dt>{$_("Next update")}</dt>
              <dd>{getHumanTime(dnsInfo.next_update)}</dd>
            </div>
          </dl>
        {/if}
      </SectionPanel>

      <SectionPanel
        title={$_("Upstream resolver")}
        description={$_("Choose the DNS server GateSentry asks after applying local records and block lists.")}
      >
        <ConnectedSettingInput
          keyName="dns_resolver"
          title={$_("DNS resolver")}
          labelText={$_("DNS resolver")}
          type="text"
          helperText={$_("Enter an IP address or resolver address reachable from this gateway.")}
        />
      </SectionPanel>

      <DnsArecords on:updatednsinfo={onUpdateDnsInfo} />
    </div>

    <Dnslists on:updatednsinfo={onUpdateDnsInfo} />
  </div>
</PageShell>

<style>
  .dns-grid,
  .dns-column {
    display: grid;
    min-width: 0;
    gap: 1rem;
  }

  .dns-grid {
    grid-template-columns: minmax(18rem, 0.8fr) minmax(24rem, 1.2fr);
    align-items: start;
  }

  .status-list {
    display: grid;
    gap: 1px;
    margin: 0;
    background: var(--cds-border-subtle, #c6c6c6);
  }

  .status-list > div {
    display: grid;
    grid-template-columns: minmax(8rem, 0.75fr) minmax(0, 1.25fr);
    gap: 1rem;
    padding: 0.875rem 1rem;
    background: var(--cds-layer, #ffffff);
  }

  .status-list dt {
    color: var(--cds-text-secondary, #525252);
  }

  .status-list dd {
    min-width: 0;
    margin: 0;
    overflow-wrap: anywhere;
  }

  @media (max-width: 65.99rem) {
    .dns-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 42rem) {
    .status-list > div {
      grid-template-columns: 1fr;
      gap: 0.25rem;
    }
  }
</style>
