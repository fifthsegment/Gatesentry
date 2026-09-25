import { expect, test } from "vitest";

import certificateSource from "../connectedCertificateComposed.svelte?raw";
import generalSettingSource from "../connectedGeneralSettingInputs.svelte?raw";
import httpsToggleSource from "../httpsToggle.svelte?raw";
import settingsSource from "../../routes/settings/settings.svelte?raw";

const settings = settingsSource.replace(/\s+/g, " ");

test("settings uses four route-local Carbon tabs", () => {
  expect(settings).toContain('<Tabs type="container" autoWidth');
  expect(settings).toContain('<Tab label={$_("General")} />');
  expect(settings).toContain('<Tab label={$_("HTTPS inspection")} />');
  expect(settings).toContain('<Tab label={$_("Connectivity")} />');
  expect(settings).toContain('<Tab label={$_("Diagnostics")} />');
  expect(settings.match(/<TabContent>/g)).toHaveLength(4);
});

test("general settings includes Raspberry Pi RAM-log guidance and readonly identity", () => {
  expect(settings).toContain("Raspberry Pi storage guidance");
  expect(settings).toContain('set the log location to "/tmp/log.db"');
  expect(settings).toContain("history is cleared whenever the device restarts");
  expect(settings).toContain("value={$store.api.username}");
  expect(settings).toContain("disabled");
});

test("support bundle preview is readonly code rather than an editable field", () => {
  expect(settings).toContain('<pre aria-labelledby="bundle-preview-label">');
  expect(settings).toContain("{bundlePreview}</code");
  expect(settings).not.toContain('<textarea id="bundle-preview"');
  expect(settings).toContain("Read-only JSON");
});

test("certificate editor separates public certificate and private key", () => {
  expect(certificateSource).toContain("Certificate (PEM)");
  expect(certificateSource).toContain("Private key (PEM)");
  expect(certificateSource).toContain('section class="private-key"');
  expect(certificateSource).toContain("Never install or share this value");
  expect(certificateSource).toContain('settingName="capem"');
  expect(certificateSource).toContain('settingName="keypem"');
});

test("scoped setting controls expose progress and feedback", () => {
  expect(generalSettingSource).toContain("InlineLoading");
  expect(generalSettingSource).toContain('$_("Loading setting…")');
  expect(generalSettingSource).toContain('$_("Saving setting…")');
  expect(generalSettingSource).toContain("createNotificationError");
  expect(httpsToggleSource).toContain(
    "preClickEvent={reviewPrivacyBeforeEnable}",
  );
  expect(httpsToggleSource).toContain("on:close={() => (modalOpen = false)}");
});

test("settings constrains tab panels and wide content", () => {
  expect(settings).toContain("overflow-x: clip");
  expect(settings).toContain("max-width: 100%");
  expect(settings).toContain("min-width: 0");
  expect(settings).toContain(".bundle-preview pre");
  expect(settings).toContain("overflow: auto");
});
