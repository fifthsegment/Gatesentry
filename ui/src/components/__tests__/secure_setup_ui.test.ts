import { compile } from "svelte/compiler";
import { expect, test } from "vitest";
import setupSource from "../../routes/setup/setup.svelte?raw";
import loginSource from "../../routes/login/login.svelte?raw";
import appSource from "../../App.svelte?raw";
import generalSettingsSource from "../connectedGeneralSettingInputs.svelte?raw";

test("setup page contains one-time credential form", () => {
  const { js } = compile(setupSource, { generate: "dom" });
  expect(js.code).toContain("/api/setup");
  expect(setupSource).toContain("Confirm password");
  expect(setupSource).toContain("Bootstrap authorization");
  expect(setupSource).toContain("!statusLoaded");
  expect(setupSource).toContain('window.location.assign(getBasePath() + "/login")');
});

test("login never persists administrator password", () => {
  expect(loginSource).not.toContain(`localStorage.setItem("password"`);
  expect(loginSource).not.toContain(`localStorage.getItem("password"`);
  expect(loginSource).toContain("/api/setup/status");
  expect(appSource).toContain('gsNavigate("/setup")');
  expect(generalSettingsSource).toContain(
    'keyName === "admin_password" || keyName === "admin_username"',
  );
  expect(generalSettingsSource).toContain('gsNavigate("/login")');
});
