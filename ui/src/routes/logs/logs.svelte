<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Grid,
    InlineNotification,
    Row,
    Search,
    Tag,
  } from "carbon-components-svelte";

  import { format } from "timeago.js";
  import { store } from "../../store/apistore";
  import _ from "lodash";
  import { onDestroy, onMount } from "svelte";
  let search = "";
  let interval = null;
  let logs = [];
  let logsToRender = [];

  const queryParams = () => {
    if (search.length > 0) {
      return `?search=${search}`;
    }
    return "";
  };

  const loadAPIData = () => {
    $store.api.doCall("/logs/viewlive" + queryParams()).then(function (json) {
      logs = JSON.parse(json.Items) as Array<any>;
      // if (search.length > 0) return;
      logsToRender = [...logs.slice(0, 30).map(itemToDataItem)];
    });
  };

  const itemToDataItem = (item, index) => ({
    id: item.ip + item.time + index + item.url,
    ip: item.ip,
    time: format(item.time * 1000),
    url: _.truncate(item.url, { length: 50 }),
    proxyResponseType:
      item.type === "dns"
        ? item.dnsResponseType || "dns"
        : item.proxyResponseType || "",
  });

  const clearSearch = () => {
    search = "";
    logsToRender = [...logs.slice(0, 30).map(itemToDataItem)];
  };

  const startInterval = () => {
    if (interval) clearInterval(interval);
    interval = setInterval(loadAPIData, 5000);
  };

  $: {
    if (search.length > 0) {
      clearInterval(interval);
      loadAPIData();
      startInterval();
    } else {
      loadAPIData();
      startInterval();
    }
  }

  // $: {
  //   logsToRender =
  //     search.length > 0
  //       ? [
  //           ...logs
  //             .filter(
  //               (item) => item.url.includes(search) || item.ip.includes(search),
  //             )
  //             .map((item, index) => itemToDataItem(item, index)),
  //         ]
  //       : logsToRender;
  //   if (search.length > 0) {
  //     clearInterval(interval);
  //   } else {
  //     startInterval();
  //   }
  // }

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  onMount(() => {
    loadAPIData();
    startInterval();
  });
</script>

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Logs</BreadcrumbItem>
    </Breadcrumb>
    <h2>Log viewer</h2>
  </Column>
</Row>
<Row>
  <Column>
    <div style="margin: 20px 0px;">
      Shows the past few requests to GateSentry.
    </div>
    <InlineNotification
      kind="info"
      lowContrast
      title="Raspberry Pi / SD Card users:"
      subtitle="To reduce SD card wear, change the log file location to RAM by going to Settings and setting the log location to &quot;/tmp/log.db&quot;. Logs in RAM will not survive a reboot."
      hideCloseButton
    />
    <div>
      <Search bind:value={search} on:clear={clearSearch} />
      <br />
      <DataTable
        sortable
        size="medium"
        style="width:100%; min-height: 600px;"
        headers={[
          {
            key: "ip",
            value: "IP",
          },
          {
            key: "time",
            value: "Time",
          },
          {
            key: "url",
            value: "URL",
          },
          {
            key: "proxyResponseType",
            value: "Response Type",
          },
        ]}
        rows={logsToRender}
      >
        <svelte:fragment slot="cell" let:cell>
          {#if cell.key === "proxyResponseType" && cell.value}
            {#if cell.value === "blocked"}
              <Tag type="red" size="sm">{cell.value}</Tag>
            {:else if cell.value === "cached"}
              <Tag type="teal" size="sm">{cell.value}</Tag>
            {:else if cell.value === "forward"}
              <Tag type="blue" size="sm">{cell.value}</Tag>
            {:else}
              <Tag type="gray" size="sm">{cell.value}</Tag>
            {/if}
          {:else}
            {cell.value}
          {/if}
        </svelte:fragment>
      </DataTable>
    </div>
  </Column>
</Row>
