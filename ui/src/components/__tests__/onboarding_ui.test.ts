import { expect, test } from "vitest";
import homeSource from "../../routes/home/home.svelte?raw";

test("home page says what GateSentry is and renders readiness with Carbon components", () => {
  expect(homeSource).toContain("/onboarding/status");
  expect(homeSource).toContain("self-hosted internet safety gateway");
  expect(homeSource).toContain("ProgressIndicator");
  expect(homeSource).toContain("ProgressStep");
  expect(homeSource).toContain("StructuredList");
  expect(homeSource).not.toContain("simple-border");
});

test("home page separates a running server from verified protection", () => {
  expect(homeSource).toContain("/onboarding/protection-check");
  expect(homeSource).toContain("Run protection check");
  expect(homeSource).toContain("controlled test domain");
  expect(homeSource).not.toContain("Startup is not protection");
  expect(homeSource).not.toContain("≤ 10");
});

test("home page summarises recent activity and HTTPS inspection state", () => {
  expect(homeSource).toContain("/decisions/summary?days=1");
  expect(homeSource).toContain("/settings/enable_https_filtering");
  expect(homeSource).not.toContain("Why do we need MITM filtering?");
});

test("onboarding renders actionable failures", () => {
  expect(homeSource).toContain(".bind.action");
  expect(homeSource).toContain(".resolver.action");
  expect(homeSource).toContain(".blocklist.action");
  expect(homeSource).toContain("InlineNotification");
});

import statsSource from "../../routes/stats/stats.svelte?raw";

test("stats page reads the decision summary with a selectable window", () => {
  expect(statsSource).toContain("/decisions/summary?days=");
  expect(statsSource).toContain("timeline");
  expect(statsSource).toContain("blocks_by_policy");
  expect(statsSource).not.toContain("/stats/byUrl");
});
