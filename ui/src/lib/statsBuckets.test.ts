import { describe, expect, test } from "vitest";
import {
  dnsChartFromSeries,
  proxyChartFromSeries,
  rebucketKey,
  type StatsSeries,
} from "./statsBuckets";

function series(): StatsSeries {
  return {
    minutes: [
      { t: "2026-09-20T10:00", dns_all: 10, dns_blocked: 2, proxy_allowed: 4, proxy_blocked: 1 },
      { t: "2026-09-20T10:01", dns_all: 5, dns_blocked: 1, proxy_allowed: 2, proxy_blocked: 0 },
      { t: "2026-09-21T11:00", dns_all: 3, dns_blocked: 0, proxy_allowed: 1, proxy_blocked: 1 },
    ],
    hourly_hosts: {
      "2026-09-20T10": {
        all: [
          { host: "a.example", count: 12 },
          { host: "b.example", count: 3 },
        ],
        blocked: [{ host: "ads.example", count: 3 }],
      },
      "2026-09-21T11": {
        all: [{ host: "a.example", count: 3 }],
        blocked: [],
      },
    },
  };
}

describe("rebucketKey", () => {
  test("7d and 24h roll minutes into hours", () => {
    expect(rebucketKey("2026-09-20T10:01", "7d")).toBe("2026-09-20T10");
    expect(rebucketKey("2026-09-20T10:01", "24h")).toBe("2026-09-20T10");
  });
  test("1h keeps the minute", () => {
    expect(rebucketKey("2026-09-20T10:01", "1h")).toBe("2026-09-20T10:01");
  });
});

describe("dnsChartFromSeries", () => {
  test("7d produces hourly totals", () => {
    const now = new Date("2026-09-21T12:00:00").getTime();
    const { chartData, topAll, topBlocked } = dnsChartFromSeries(
      series(),
      "7d",
      [],
      now,
    );
    const hour10 = chartData.filter(
      (p) => p.date.getTime() === new Date("2026-09-20T10:00:00").getTime(),
    );
    const all = hour10.find((p) => p.group === "All Requests");
    const blocked = hour10.find((p) => p.group === "Blocked Requests");
    expect(all?.value).toBe(15);
    expect(blocked?.value).toBe(3);
    expect(topAll[0]).toEqual({ host: "a.example", count: 15 });
    expect(topBlocked[0]).toEqual({ host: "ads.example", count: 3 });
  });

  test("1h keeps per-minute points", () => {
    const now = new Date("2026-09-20T10:30:00").getTime();
    const { chartData } = dnsChartFromSeries(series(), "1h", [], now);
    const minutes = chartData.filter((p) => p.group === "All Requests");
    expect(minutes).toHaveLength(2);
    expect(minutes.map((p) => p.value).sort((a, b) => a - b)).toEqual([5, 10]);
  });
});

describe("proxyChartFromSeries", () => {
  test("rolls allowed/blocked into hours for 7d", () => {
    const now = new Date("2026-09-21T12:00:00").getTime();
    const data = proxyChartFromSeries(series(), "7d", now);
    const hour10 = data.filter(
      (p) => p.date.getTime() === new Date("2026-09-20T10:00:00").getTime(),
    );
    expect(hour10.find((p) => p.group === "Allowed")?.value).toBe(6);
    expect(hour10.find((p) => p.group === "Blocked")?.value).toBe(1);
  });
});
