<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Grid,
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
    proxyResponseType: item.proxyResponseType,
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

<Grid>
  <Row>
    <Column>
      <Breadcrumb style="margin-bottom: 10px;">
        <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
        <BreadcrumbItem href="/logs">Logs</BreadcrumbItem>
      </Breadcrumb>
      <h2>Log viewer</h2>
    </Column>
  </Row>
  <Row>
    <Column>
      <div style="margin: 20px 0px;">
        Shows the past few requests to GateSentry.
      </div>
      <div style="margin-bottom: 15px;">
        <Tag>
          IMPORTANT: If you are using GateSentry on a Raspberry Pi please make
          sure to change GateSentry's log file location to RAM. You can do that
          by going to Settings and changing the log file location to
          "/tmp/log.db".
        </Tag>
      </div>
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
        ></DataTable>
      </div>
    </Column>
  </Row>
</Grid>
