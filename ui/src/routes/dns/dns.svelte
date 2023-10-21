<script lang="ts">
  import Dnslists from "../../components/dnslists.svelte";
  import DnsArecords from "../../components/dnsArecords.svelte";
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    Grid,
    Row,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { set } from "lodash";
  import { Restart } from "carbon-icons-svelte";
  let dnsInfo = null;

  const loadDnsInfo = async () => {
    dnsInfo = null;
    dnsInfo = await $store.api.doCall("/dns/info");
  };
  onMount(async () => {
    await loadDnsInfo();
  });

  const getHumanTime = (time) => {
    const date = new Date(time * 1000);
    return date.toLocaleString();
  };

  const onUpdateDnsInfo = async () => {
    dnsInfo = null;
    setTimeout(async () => {
      await loadDnsInfo();
    }, 3000);
  };
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>DNS Server</BreadcrumbItem>
</Breadcrumb>
<h2>DNS Server</h2>
<br />

<Row>
  <Column sm={5} md={5} lg={5}>
    <h5>{$_("DNS Info")}</h5>

    {#if dnsInfo == null}
      <div class="simple-border">
        {$_("Loading...")}
      </div>
    {:else}
      <div class="simple-border" style="font-size:0.95em; position: relative;">
        <span
          style="position: absolute; right: 5px; top: 5px; cursor:pointer; color: gray;"
          on:click={loadDnsInfo}
          ><Restart />
        </span>
        <div class="info-item">
          {$_("Blocked domains")}
          <label class="bx--label">{dnsInfo?.number_domains_blocked} </label>
        </div>
        <div class="info-item">
          {$_("Last updated")}
          <label class="bx--label">
            {getHumanTime(dnsInfo?.last_updated)}</label
          >
        </div>
        <div class="info-item">
          {$_("Next update")}
          <label class="bx--label">{getHumanTime(dnsInfo?.next_update)}</label>
        </div>
      </div>
    {/if}
    <br />
    <DnsArecords on:updatednsinfo={onUpdateDnsInfo} />
  </Column>
  <Column sm={11} md={11} lg={11}>
    <Dnslists on:updatednsinfo={onUpdateDnsInfo} />
  </Column>
</Row>

<style>
  .info-item {
    padding-bottom: 4px;
    margin: 0px;
  }

  .info-item label {
    padding-bottom: 0;
    margin-bottom: 0;
  }
</style>
