import { expect, test } from "vitest";
import httpsToggleSource from "../httpsToggle.svelte?raw";
import inspectionStatusSource from "../inspectionStatus.svelte?raw";
import settingsSource from "../../routes/settings/settings.svelte?raw";

test("httpsToggle shows consent gate before enabling inspection", () => {
  expect(httpsToggleSource).toContain("consentAccepted");
  expect(httpsToggleSource).toContain("Accept consent and enable inspection");
  expect(httpsToggleSource).toContain("primaryButtonDisabled={!consentAccepted}");
  expect(httpsToggleSource).toContain("preventCloseOnClickOutside");
  expect(httpsToggleSource).toContain("on:toggle=");
  expect(httpsToggleSource).toContain("on:close={cancelModal}");
});

test("httpsToggle links platform certificate instructions to docs", () => {
  expect(httpsToggleSource).toContain("Platform certificate instructions");
  expect(httpsToggleSource).toContain("https-inspection.md");
  expect(httpsToggleSource).toContain("iOS / iPadOS");
  expect(httpsToggleSource).toContain("macOS");
  expect(httpsToggleSource).toContain("Windows");
  expect(httpsToggleSource).toContain("Android");
  expect(httpsToggleSource).toContain("Linux system store");
  expect(httpsToggleSource).toContain("Firefox");
});

test("httpsToggle includes a verification step", () => {
  expect(httpsToggleSource).toContain("Verification");
  expect(httpsToggleSource).toContain("Inspect the certificate chain");
  expect(httpsToggleSource).toContain("inspect count should");
  expect(httpsToggleSource).toContain("bypass count should");
});

test("inspectionStatus panel shows enabled state and certificate expiry", () => {
  expect(inspectionStatusSource).toContain("/certificate/inspection");
  expect(inspectionStatusSource).toContain("enabled");
  expect(inspectionStatusSource).toContain("expired");
  expect(inspectionStatusSource).toContain("not_after");
});

test("inspectionStatus panel shows exclusions with bypass wording", () => {
  expect(inspectionStatusSource).toContain("exclusions");
  expect(inspectionStatusSource).toContain("exclusion_note");
  expect(inspectionStatusSource).toContain("tunnelled without inspection");
});

test("inspectionStatus panel shows inspect-vs-bypass counts with caveat", () => {
  expect(inspectionStatusSource).toContain("inspect_count");
  expect(inspectionStatusSource).toContain("bypass_count");
  expect(inspectionStatusSource).toContain("coverage_caveat");
});

test("settings page wires in the inspection status panel", () => {
  expect(settingsSource).toContain("InspectionStatus");
  expect(settingsSource).toContain("inspectionStatus.svelte");
  expect(settingsSource).toContain("<InspectionStatus");
});
