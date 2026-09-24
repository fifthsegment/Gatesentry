import { expect, test } from "vitest";
import logsSource from "../../routes/logs/logs.svelte?raw";
import statsSource from "../../routes/stats/stats.svelte?raw";

test("logs enrich client IPs from an independently refreshed device list", () => {
  expect(logsSource).toContain('$store.api.doCall("/devices")');
  expect(logsSource).toContain("buildDeviceNameMap");
  expect(logsSource).toContain("formatDeviceAddress");
  expect(logsSource).toContain("setTimeout(loadDevices, 60000)");
  expect(logsSource).toContain("setTimeout(loadDecisions, 5000)");
  expect(logsSource).toContain("clearTimeout(deviceTimer)");
  expect(logsSource).toContain("decisionRefreshPending = true");
  expect(logsSource).toContain("query === buildQuery()");
});

test("stats reports process-lifetime proxy traffic on a live poll", () => {
  expect(statsSource).toContain('$store.api.doCall("/proxy/traffic")');
  expect(statsSource).toContain("setTimeout(loadTraffic, 5000)");
  expect(statsSource).toContain("Since GateSentry started");
  expect(statsSource).toContain("clearTimeout(trafficTimer)");
  expect(statsSource).toContain("summaryRefreshPending = true");
  expect(statsSource).toContain("requested === selected");
  expect(statsSource).toContain("setTimeout(load, 30000)");
});
