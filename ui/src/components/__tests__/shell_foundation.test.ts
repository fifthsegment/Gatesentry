import { expect, test } from "vitest";
import appSource from "../../App.svelte?raw";
import menuSource from "../../menu.ts?raw";
import mainSource from "../../main.ts?raw";
import logsSource from "../../routes/logs/logs.svelte?raw";

const compact = (source: string) => source.replace(/\s+/g, " ");

test("protected routes mount only after setup and token verification", () => {
  const source = compact(appSource);
  expect(source).toContain('state = "checking"');
  expect(source).toContain("await $store.api.verifyToken()");
  expect(source).toContain('{:else if state === "login"}');
  expect(source).toContain("{:else} <div class=\"app-frame\">");
  expect(source.indexOf('state = "authenticated"')).toBeLessThan(
    source.indexOf('<Route path="/users"'),
  );
});

test("authentication routes use a shell without protected chrome", () => {
  expect(appSource).toContain('<Route path="/setup" component={Setup} />');
  expect(appSource).toContain('<Route path="/login">');
  expect(appSource).toContain('<div class="app-frame">');
  expect(appSource).not.toContain('<Globalheader bind:isSideNavOpen bind:userProfilePanelOpen />');
});

test("navigation is typed, base-path aware, active, and includes exclude URLs", () => {
  expect(menuSource).toContain("export type MenuItem");
  expect(menuSource).toContain('href: "/excludeurls"');
  expect(menuSource).toContain("getBasePath() + href");
  expect(menuSource).toContain("isRouteActive");
});

test("Carbon styles load before application overrides", () => {
  expect(mainSource.indexOf('carbon-components-svelte/css/g10.css')).toBeLessThan(
    mainSource.indexOf('./app.css'),
  );
});

test("Raspberry Pi storage guidance is not shown on the decision log", () => {
  expect(logsSource).not.toContain("Raspberry Pi");
  expect(logsSource).not.toContain("/tmp/log.db");
});
