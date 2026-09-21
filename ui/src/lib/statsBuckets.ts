/** Compact stats series from GET /api/stats/byUrl and /api/stats/proxy. */

export type MinutePoint = {
  t: string;
  dns_all?: number;
  dns_blocked?: number;
  proxy_allowed?: number;
  proxy_blocked?: number;
  proxy_ssl_bump?: number;
  proxy_ssl_direct?: number;
};

export type HostCount = { host: string; count: number };

export type HourUser = {
  user: string;
  total: number;
  allowed: number;
  blocked: number;
};

export type HourHostSet = {
  all?: HostCount[];
  blocked?: HostCount[];
  proxy_allowed?: HostCount[];
  proxy_blocked?: HostCount[];
  users?: HourUser[];
};

export type StatsSeries = {
  minutes: MinutePoint[];
  hourly_hosts: Record<string, HourHostSet>;
};

export type ScaleId = "7d" | "24h" | "1h";

export function scaleCutoffMs(scale: ScaleId, now = Date.now()): number {
  if (scale === "1h") return now - 3_600_000;
  if (scale === "24h") return now - 86_400_000;
  return now - 7 * 86_400_000;
}

/** Minute key "YYYY-MM-DDTHH:MM" → hour "YYYY-MM-DDTHH" or the minute itself. */
export function rebucketKey(minuteKey: string, scale: ScaleId): string {
  if (scale === "1h") return minuteKey;
  if (minuteKey.length >= 13) return minuteKey.slice(0, 13);
  return minuteKey;
}

export function minuteKeyToMs(minuteKey: string): number {
  const d = new Date(minuteKey.length === 16 ? minuteKey + ":00" : minuteKey);
  return d.getTime();
}

export function parseBucketDate(key: string, scale: ScaleId): Date {
  if (scale === "1h") return new Date(key + ":00");
  if (key.length >= 13) return new Date(key + ":00:00");
  return new Date(key + ":00:00");
}

export type ChartPoint = { group: string; date: Date; value: number };

/** Roll a minute series into the visible window for the DNS traffic chart. */
export function dnsChartFromSeries(
  series: StatsSeries | null,
  scale: ScaleId,
  extra: { ts: number; domain: string; blocked: boolean }[] = [],
  now = Date.now(),
): { chartData: ChartPoint[]; topAll: HostCount[]; topBlocked: HostCount[] } {
  const cutoff = scaleCutoffMs(scale, now);
  const allMap = new Map<string, number>();
  const blockedMap = new Map<string, number>();

  if (series?.minutes) {
    for (const p of series.minutes) {
      const ts = minuteKeyToMs(p.t);
      if (ts < cutoff || ts > now) continue;
      const key = rebucketKey(p.t, scale);
      allMap.set(key, (allMap.get(key) || 0) + (p.dns_all || 0));
      blockedMap.set(
        key,
        (blockedMap.get(key) || 0) + (p.dns_blocked || 0),
      );
    }
  }

  for (const evt of extra) {
    if (evt.ts < cutoff) continue;
    const d = new Date(evt.ts);
    const min = localMinuteKey(d);
    const key = rebucketKey(min, scale);
    allMap.set(key, (allMap.get(key) || 0) + 1);
    if (evt.blocked) {
      blockedMap.set(key, (blockedMap.get(key) || 0) + 1);
    }
  }

  const keys = new Set([...allMap.keys(), ...blockedMap.keys()]);
  const sorted = [...keys].sort();
  const chartData: ChartPoint[] = [];
  for (const k of sorted) {
    const date = parseBucketDate(k, scale);
    if (allMap.has(k)) {
      chartData.push({ group: "All Requests", date, value: allMap.get(k)! });
    }
    if (blockedMap.has(k)) {
      chartData.push({
        group: "Blocked Requests",
        date,
        value: blockedMap.get(k)!,
      });
    }
  }

  return {
    chartData,
    topAll: mergeHourlyHosts(series, cutoff, "all", now),
    topBlocked: mergeHourlyHosts(series, cutoff, "blocked", now),
  };
}

export function proxyChartFromSeries(
  series: StatsSeries | null,
  scale: ScaleId,
  now = Date.now(),
): ChartPoint[] {
  const cutoff = scaleCutoffMs(scale, now);
  const allowed = new Map<string, number>();
  const blocked = new Map<string, number>();
  if (series?.minutes) {
    for (const p of series.minutes) {
      const ts = minuteKeyToMs(p.t);
      if (ts < cutoff || ts > now) continue;
      const key = rebucketKey(p.t, scale);
      allowed.set(key, (allowed.get(key) || 0) + (p.proxy_allowed || 0));
      blocked.set(key, (blocked.get(key) || 0) + (p.proxy_blocked || 0));
    }
  }
  const keys = [...new Set([...allowed.keys(), ...blocked.keys()])].sort();
  const chartData: ChartPoint[] = [];
  for (const k of keys) {
    const date = parseBucketDate(k, scale);
    chartData.push({ group: "Allowed", date, value: allowed.get(k) || 0 });
    chartData.push({ group: "Blocked", date, value: blocked.get(k) || 0 });
  }
  return chartData;
}

function mergeHourlyHosts(
  series: StatsSeries | null,
  cutoff: number,
  field: "all" | "blocked" | "proxy_allowed" | "proxy_blocked",
  now = Date.now(),
): HostCount[] {
  const counts = new Map<string, number>();
  if (!series?.hourly_hosts) return [];
  for (const [hour, set] of Object.entries(series.hourly_hosts)) {
    const hourMs = new Date(hour + ":00:00").getTime();
    if (hourMs + 3_600_000 < cutoff || hourMs > now) continue;
    const list = set[field] || [];
    for (const h of list) {
      counts.set(h.host, (counts.get(h.host) || 0) + h.count);
    }
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5)
    .map(([host, count]) => ({ host, count }));
}

export type ProxyWindow = {
  total: number;
  allowed: number;
  blocked: number;
  ssl_bumped: number;
  ssl_direct: number;
  topAllowed: HostCount[];
  topBlocked: HostCount[];
};

export function proxyWindowFromSeries(
  series: StatsSeries | null,
  scale: ScaleId,
  now = Date.now(),
): ProxyWindow {
  const cutoff = scaleCutoffMs(scale, now);
  let allowed = 0;
  let blocked = 0;
  let sslBumped = 0;
  let sslDirect = 0;
  if (series?.minutes) {
    for (const p of series.minutes) {
      const ts = minuteKeyToMs(p.t);
      if (ts < cutoff || ts > now) continue;
      allowed += p.proxy_allowed || 0;
      blocked += p.proxy_blocked || 0;
      sslBumped += p.proxy_ssl_bump || 0;
      sslDirect += p.proxy_ssl_direct || 0;
    }
  }
  return {
    total: allowed + blocked,
    allowed,
    blocked,
    ssl_bumped: sslBumped,
    ssl_direct: sslDirect,
    topAllowed: mergeHourlyHosts(series, cutoff, "proxy_allowed", now),
    topBlocked: mergeHourlyHosts(series, cutoff, "proxy_blocked", now),
  };
}

export function localMinuteKey(d: Date): string {
  const Y = d.getFullYear();
  const M = String(d.getMonth() + 1).padStart(2, "0");
  const D = String(d.getDate()).padStart(2, "0");
  const h = String(d.getHours()).padStart(2, "0");
  const m = String(d.getMinutes()).padStart(2, "0");
  return `${Y}-${M}-${D}T${h}:${m}`;
}
