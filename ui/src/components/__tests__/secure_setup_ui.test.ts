import { compile } from "svelte/compiler";
import { expect, test } from "vitest";
import setupSource from "../../routes/setup/setup.svelte?raw";
import loginSource from "../../routes/login/login.svelte?raw";
import appSource from "../../App.svelte?raw";
import generalSettingsSource from "../connectedGeneralSettingInputs.svelte?raw";
import headerSource from "../headerrightnav.svelte?raw";
import globalHeaderSource from "../globalheader.svelte?raw";
import settingsSource from "../../routes/settings/settings.svelte?raw";
import certificateLinkSource from "../downloadCertificateLink.svelte?raw";

test("setup page contains one-time credential form", () => {
  const { js } = compile(setupSource, { generate: "dom" });
  expect(js.code).toContain("/api/setup");
  expect(setupSource).toContain("Confirm password");
  // Setup asks for credentials only; there is no authorization step to satisfy.
  expect(setupSource).not.toContain("Bootstrap authorization");
  expect(setupSource).not.toContain("requires_authorization");
  expect(setupSource).toContain("!statusLoaded");
  expect(setupSource).toContain("passwordHasValidByteLength");
  expect(setupSource).toContain("UTF-8 bytes");
  expect(setupSource).not.toContain("maxlength={72}");
  expect(setupSource).toContain('window.location.assign(getBasePath() + "/login")');
});

test("login never persists administrator password", () => {
  expect(loginSource).not.toContain(`localStorage.setItem("password"`);
  expect(loginSource).not.toContain(`localStorage.getItem("password"`);
  expect(loginSource).toContain(`localStorage.removeItem("password")`);
  expect(loginSource).toContain(`localStorage.removeItem("rememberMe")`);
  expect(loginSource).not.toContain("/api/setup/status");
  expect(appSource).toContain('fetch(getBasePath() + "/api/setup/status")');
  expect(appSource).toContain('gsNavigate("/setup", { replace: true })');
  expect(appSource).toContain('state = "error"');
  expect(appSource).toContain("Retry status check");
  expect(generalSettingsSource).toContain(
    'keyName === "admin_password" || keyName === "admin_username"',
  );
  expect(generalSettingsSource).toContain('gsNavigate("/login")');
});

test("certificate download stays on the configured GateSentry base path", () => {
  expect(certificateLinkSource).toContain(
    'getBasePath() + "/api/files/certificate"',
  );
  expect(certificateLinkSource).toContain("<Button");
  expect(certificateLinkSource).toContain("download");
  expect(certificateLinkSource).not.toContain('target="_blank"');
});

test("authenticated administrator identity is displayed from the verified session", () => {
  expect(headerSource).not.toContain("Logged in as admin");
  expect(headerSource).toContain("$store.api.username");
  expect(loginSource).toContain('data.Username || ""');
  expect(loginSource).not.toContain("data.Username || username");
  expect(settingsSource).toContain("value={$store.api.username}");
  expect(settingsSource).not.toContain('keyName="admin_username"');
});

test("login keeps clear and submit actions in one row", () => {
  expect(loginSource).toContain("<ButtonSet>");
  expect(loginSource).not.toContain("<ButtonSet stacked>");
});

test("logout signals the shell so the login route renders immediately", () => {
  expect(headerSource).toContain('dispatch("loggedout")');
  expect(globalHeaderSource).toContain('on:loggedout={() => dispatch("loggedout")}');
  expect(appSource).toContain('on:loggedout={() => (state = "login")}');
});
