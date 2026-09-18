<script lang="ts">
  import {
    Breadcrumb,
    BreadcrumbItem,
    Column,
    DataTable,
    Grid,
    Loading,
    Row,
    Tag,
  } from "carbon-components-svelte";
  import "@carbon/charts/styles.css";
  import { AreaChart } from "@carbon/charts";
  import { onDestroy, onMount } from "svelte";
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
  type DecisionSummary = {
    window: string;
    total: number;
    by_action: Record<string, number>;
    by_layer: Record<string, number>;
    top_blocked_domains: { key: string; count: number }[];
    top_reasons: { key: string; count: number }[];
    affected_devices: { ip: string; device: string; count: number }[];
    inspection_failures: number;
  };
  let interval: ReturnType<typeof setInterval> | null = null;
  const options = {
    title: "Requests served",
    axes: {
      bottom: {
        title: "DNS + Proxy Requests in the past week",
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

  let chart: any;
  let chartHolder: HTMLElement;
  let data: any[] = [];
  let responseData: ResponseData = null;
  let summary: DecisionSummary | null = null;

  const updateChartData = async () => {
    try {
      const json = (await $store.api.doCall("/stats/byUrl")) as ResponseData;
      if (!json) return;
      responseData = json;
      data =
        json.blocked && json.all
          ? [
              ...Object.entries(json["blocked"]).map(([key, item]) => {
                return {
                  group: "Blocked Requests",
                  date: new Date(key).toISOString(),
                  value: item.total,
                };
              }),
              ...Object.entries(json["all"]).map(([key, item]) => {
                return {
                  group: "All Requests",
                  date: new Date(key).toISOString(),
                  value: item.total,
                };
              }),
            ]
          : [];
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

  const loadSummary = async () => {
    try {
      summary = await $store.api.doCall("/decisions/summary");
    } catch (error) {
      console.error("Error fetching decision summary:", error);
    }
  };

  onMount(() => {
    chartHolder = document.getElementById("statschart");
    if (!chartHolder) throw new Error("Could not find chart holder element");
    // @ts-ignore
    interval = setInterval(() => {
      updateChartData();
      loadSummary();
    }, 5000);
    updateChartData();
    loadSummary();
  });

  onDestroy(() => {
    if (interval) clearInterval(interval);
    if (chart) chart.destroy();
  });
</script>

<Row>
  <Column>
    <Breadcrumb style="margin-bottom: 10px;">
      <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
      <BreadcrumbItem>Stats</BreadcrumbItem>
    </Breadcrumb>
    <h2>Stats</h2>
    <div id="statschart"></div>
  </Column>
</Row>
{#if chart}
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
{/if}
{#if !chart}
  <Row>
    <Column>
      <Loading />
    </Column>
  </Row>
{/if}

{#if summary}
  <Row style="margin-top: 30px;">
    <Column>
      <h3>Decision summary — last {summary.window}</h3>
      <div style="margin: 15px 0;">
        <Tag>Decisions: {summary.total}</Tag>
        {#if summary.inspection_failures > 0}
          <Tag type="red">Inspection failures: {summary.inspection_failures}</Tag>
        {/if}
      </div>
      <div style="margin-bottom: 10px;">
        <Tag type="high-contrast">
          DNS blocks return NXDOMAIN (no intercepted block page is shown to
          the user). HTTPS block pages require MITM inspection to be enabled.
        </Tag>
      </div>
    </Column>
  </Row>
  <Row>
    <Column>
      <h4>Frequently blocked domains</h4>
      {#if summary.top_blocked_domains && summary.top_blocked_domains.length > 0}
        <DataTable
          headers={[
            { key: "key", value: "Domain" },
            { key: "count", value: "Blocks" },
          ]}
          rows={summary.top_blocked_domains.map((d, i) => ({
            id: d.key + i,
            key: d.key,
            count: d.count,
          }))}
        />
      {:else}
        <p><i>No blocked domains in this window.</i></p>
      {/if}
    </Column>
    <Column>
      <h4>Most affected devices</h4>
      {#if summary.affected_devices && summary.affected_devices.length > 0}
        <DataTable
          headers={[
            { key: "ip", value: "IP address" },
            { key: "device", value: "Device" },
            { key: "count", value: "Decisions" },
          ]}
          rows={summary.affected_devices.map((d, i) => ({
            id: d.ip + i,
            ip: d.ip,
            device: d.device || "—",
            count: d.count,
          }))}
        />
      {:else}
        <p><i>No device activity in this window.</i></p>
      {/if}
    </Column>
  </Row>
{/if}
