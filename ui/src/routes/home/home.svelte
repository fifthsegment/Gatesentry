<script lang="ts">
  import { Column, DataTable, Grid, Row } from "carbon-components-svelte";
  import "@carbon/charts/styles.css";
  import { AreaChart } from "@carbon/charts";
  import { onMount } from "svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";

  type Keys = "blocked" | "all";
  type HostData = {
    host: string;
    count: number;
  };
  type ResponseData = {
    [key in Keys]: {
      [date: string]: {
        total: number;
        hosts: HostData[];
      };
    };
  };

  const options = {
    title: "Requests served",
    axes: {
      bottom: {
        title: "DNS Requests in the past week",
        mapsTo: "date",
        scaleType: "time",
      },
      left: {
        mapsTo: "value",
        scaleType: "linear",
      },
    },
    height: "400px",
    toolbar: {
      enabled: false,
    },
  };

  let chart;
  let chartHolder;
  let data = [];
  let responseData: ResponseData = null;

  const updateChartData = async () => {
    try {
      const json = (await $store.api.doCall("/stats/byUrl")) as ResponseData;
      responseData = json;
      data =
        json.blocked && json.all
          ? [
              ...Object.entries(json["blocked"]).map(([key, item]) => {
                return {
                  group: "Blocked Requests",
                  date: new Date(key).toISOString(), // You can adjust this as needed
                  value: item.total,
                };
              }),
              ...Object.entries(json["all"]).map(([key, item]) => {
                return {
                  group: "All Requests",
                  date: new Date(key).toISOString(), // You can adjust this as needed
                  value: item.total,
                };
              }),
            ]
          : [];
      // Process the JSON data and update the chart
      // const data = json.items.map((key, value) => ({
      //     group: "URL Counts",
      //     date: new Date(key).toISOString(), // You can adjust this as needed
      //     value: value.count,
      // }));
      if (!chart) {
        // @ts-ignore
        chart = new AreaChart(chartHolder, {
          data: data,
          // @ts-ignore
          options,
        });
        return;
      } else {
        // @ts-ignore
        chart.model.setData(data);
      }
    } catch (error) {
      console.error("Error fetching data:", error);
    }
  };

  onMount(() => {
    chartHolder = document.getElementById("statschart");
    if (!chartHolder) throw new Error("Could not find chart holder element");
    // @ts-ignore

    // Call updateChartData every 30 seconds
    setInterval(updateChartData, 5000);

    // Initial data fetch
    updateChartData();
  });
</script>

<Grid>
  <Row>
    <Column>
      <h1>Stats</h1>
      <div id="statschart"></div>
    </Column>
  </Row>
  <Row>
    <Column>
      <div>
        <br />
        <h4>{$_("Top 5 Blocked Requests")}</h4>
        <br />
        {#if responseData && responseData["blocked"]}
          <DataTable
            headers={[
              { key: "host", value: "Host" },
              { key: "count", value: "Times requested" },
            ]}
            rows={Object.entries(responseData["blocked"])
              .flatMap(([key, item]) => {
                return item.hosts.map((host) => {
                  return {
                    id: host.host,
                    host: host.host,
                    count: host.count,
                  };
                });
              })
              .slice(0, 5)}
          />
        {/if}
        {#if responseData && !responseData["blocked"]}
          <p>
            <i>{$_("Nothing found. Please make some requests.")}</i>
          </p>
        {/if}
      </div>
    </Column>
    <Column>
      <div>
        <br />
        <h4>{$_("Top 5 Requests")}</h4>
        <br />
        {#if responseData && responseData["all"]}
          <DataTable
            headers={[
              { key: "host", value: "Host" },
              { key: "count", value: "Times requested" },
            ]}
            rows={Object.entries(responseData["all"])
              .flatMap(([key, item]) => {
                return item.hosts.map((host, index) => {
                  return {
                    id: host.host + "all" + index,
                    host: host.host,
                    count: host.count,
                  };
                });
              })
              .filter(
                (item, index, self) =>
                  index === self.findIndex((t) => t.id === item.id),
              )
              .sort((a, b) => b.count - a.count)
              .slice(0, 5)}
          />
        {/if}
        {#if responseData && !responseData["all"]}
          <p>
            <i>{$_("Nothing found. Please make some requests.")}</i>
          </p>
        {/if}
      </div>
    </Column>
  </Row>
</Grid>
