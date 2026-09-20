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

test("home page distinguishes a running server from verified protection", () => {
  expect(homeSource).toContain("/onboarding/protection-check");
  expect(homeSource).toContain("Run protection check");
  expect(homeSource).toContain("Startup is not protection");
  expect(homeSource).toContain("controlled test domain");
  expect(homeSource).toContain("Time from setup start to first explained protection");
  expect(homeSource).toContain("target");
  expect(homeSource).toContain("≤ 10");
});

test("home page keeps DNS and HTTPS inspection coverage distinct", () => {
  expect(homeSource).toContain("HTTPS inspection");
  expect(homeSource).toContain("advanced");
  expect(homeSource).toContain("not required");
  expect(homeSource).toContain("remain outside HTTPS inspection");
  expect(homeSource).toContain("cannot inspect or explain URL, MIME, keyword, or image-content decisions");
  expect(homeSource).toContain("Devices that use GateSentry for DNS are the only devices covered");
  expect(homeSource).not.toContain("Why do we need MITM filtering?");
});

test("onboarding renders actionable failures", () => {
  expect(homeSource).toContain(".bind.action");
  expect(homeSource).toContain(".resolver.action");
  expect(homeSource).toContain(".blocklist.action");
  expect(homeSource).toContain("InlineNotification");
});
