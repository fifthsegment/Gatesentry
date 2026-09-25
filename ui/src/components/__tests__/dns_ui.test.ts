import { expect, test } from "vitest";

import recordsSource from "../dnsArecords.svelte?raw";
import listsSource from "../dnslists.svelte?raw";
import dnsSource from "../../routes/dns/dns.svelte?raw";
import filterSource from "../../routes/filter/filter.svelte?raw";
import editorSource from "../filtereditor.svelte?raw";

const compact = (source: string) => source.replace(/\s+/g, " ");

test("DNS page uses shared sections, semantic status refresh, and delayed reload", () => {
  const dns = compact(dnsSource);
  expect(dns).toContain("<PageShell");
  expect(dns).toContain("<SectionPanel");
  expect(dns).toContain('keyName="dns_resolver"');
  expect(dns).toContain('on:click={loadDnsInfo}');
  expect(dns).toContain("if (refreshTimer) clearTimeout(refreshTimer)");
  expect(dns).toContain("}, 3000)");
  expect(dns).toContain("onDestroy");
  expect(dns).not.toContain("simple-border");
  expect(dns).not.toContain("<span on:click");
});

test("custom DNS records preserve endpoint payload and stable edit identity", () => {
  const records = compact(recordsSource);
  expect(records).toContain('const url = "/dns/custom_entries"');
  expect(records).toContain("records.map(({ domain, ip }) => ({ domain, ip }))");
  expect(records).toContain("row.id === editingRowId");
  expect(records).toContain("isIPv4(ip)");
  expect(records).toContain("isIPv4(ipText.trim())");
  expect(records).toContain("<ConfirmDialog");
  expect(records).toContain("<ResourceState");
  expect(records).not.toContain("data[editingRowId - 1]");
  expect(records).not.toContain('style="float:right;"');
});

test("custom DNS records expose one add-record action, not a duplicate empty-state button", () => {
  expect(recordsSource.match(/Add record/g)?.length ?? 0).toBe(1);
  expect(recordsSource).toContain('slot="actions"');
  expect(recordsSource).toContain("on:click={openCreate}");
});

test("DNS block lists edit by row identity and confirm removal", () => {
  const lists = compact(listsSource);
  expect(lists).toContain('getSetting("dns_custom_entries")');
  expect(lists).toContain('setSetting( "dns_custom_entries", JSON.stringify(next)');
  expect(lists).toContain("index === editingRowId ? value : item");
  expect(lists).toContain("index !== target.id");
  expect(lists).toContain("<ConfirmDialog");
  expect(lists).toContain("<CodeSnippet");
  expect(lists).not.toContain("item == id ? editingItemValue");
  expect(lists).not.toContain("simple-border");
});

test("HTTPS routes share navigation, scope copy, and fixed filter IDs", () => {
  const filter = compact(filterSource);
  expect(filter).toContain("<PageShell");
  expect(filter).toContain('aria-label={$_("HTTPS inspection sections")}');
  expect(filter).toContain('path: "/blockedkeywords"');
  expect(filter).toContain('path: "/blockedfiletypes"');
  expect(filter).toContain('path: "/excludehosts"');
  expect(filter).toContain('path: "/excludeurls"');
  expect(filter).toContain('filterId: "bVxTPTOXiqGRbhF"');
  expect(filter).toContain('filterId: "JHGJiwjkGOeglsk"');
  expect(filter).toContain('filterId: "CeBqssmRbqXzbHR"');
  expect(filter).toContain('filterId: "JHGJiwjkGOeglsd"');
  expect(filter).toContain("Use Policies to block whole domains");
  expect(filter).not.toContain("<br />");
});

test("filter editor preserves payload casing and accepts backend response casing", () => {
  const editor = compact(editorSource);
  expect(editor).toContain("Content: item.content");
  expect(editor).toContain("Score: item.score");
  expect(editor).toContain("content?.response ?? content?.Response");
  expect(editor).toContain("<ConfirmDialog");
  expect(editor).toContain("<ResourceState");
  expect(editor).toContain("cancelEditing");
  expect(editor).not.toContain("content.Response.includes");
  expect(editor).not.toContain("<div on:click");
  expect(editor).not.toContain('style="float:right;"');
});
