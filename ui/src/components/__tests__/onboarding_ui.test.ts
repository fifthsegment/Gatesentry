import { expect, test } from "vitest";
import homeSource from "../../routes/home/home.svelte?raw";

test("home page guides DNS-first onboarding and distinguishes protection", () => {
  expect(homeSource).toContain("/onboarding/status");
  expect(homeSource).toContain("/onboarding/protection-check");
  expect(homeSource).toContain("Run protection check");
  expect(homeSource).toContain("Startup is not protection");
  expect(homeSource).toContain("controlled test domain");
  expect(homeSource).toContain("Time from setup start to first explained protection");
  expect(homeSource).toContain("target");
  expect(homeSource).toContain("≤ 10");
});

test("home page keeps DNS and HTTPS inspection coverage distinct", () => {
  expect(homeSource).toContain("Optional: HTTPS inspection");
  expect(homeSource).toContain("advanced and opt-in");
  expect(homeSource).toContain("It is not required for the steps above");
  expect(homeSource).toContain("remain outside HTTPS inspection");
  expect(homeSource).toContain("cannot inspect or explain URL, MIME, keyword, or image-content decisions");
  expect(homeSource).toContain("Devices that use GateSentry for DNS are the only devices covered");
  expect(homeSource).not.toContain("Why do we need MITM filtering?");
});

test("onboarding renders actionable failures", () => {
  expect(homeSource).toContain("status.bind.action");
  expect(homeSource).toContain("status.resolver.action");
  expect(homeSource).toContain("status.blocklist.action");
  expect(homeSource).toContain("Needs attention");
});

