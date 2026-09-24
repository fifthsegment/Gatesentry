import { describe, expect, test } from "vitest";
import page from "../../routes/users/users.svelte?raw";
import form from "../../routes/users/usermodal.svelte?raw";

describe("Proxy users Carbon workflow", () => {
  test("explains proxy users and exposes enforcement and list states", () => {
    expect(page).toContain("Proxy users");
    expect(page).toContain("separate from administrator accounts");
    expect(page).toContain('keyName="EnableUsers"');
    expect(page).toContain("Loading proxy users…");
    expect(page).toContain("No proxy users yet");
    expect(page).toContain('size="compact"');
  });

  test("uses a single modal submit owner and Carbon confirmation", () => {
    expect(page).toContain('on:submit={() => userForm?.submit()}');
    expect(page).toContain('title={$_("Delete proxy user?")}');
    expect(page).toContain("danger");
    expect(page).not.toContain("confirm(");
    expect(form).toContain("export async function submit()");
    expect(form).not.toContain("on:submit");
    expect(form).not.toContain('type="submit"');
  });

  test("preserves API operations and denied-by-default creation", () => {
    expect(form).toContain('let allowAccess = "false"');
    expect(form).toContain("$store.api.createUser(payload)");
    expect(form).toContain("$store.api.updateUser(payload)");
    expect(page).toContain("$store.api.updateUser({");
    expect(page).toContain("$store.api.deleteUser(username)");
    expect(form).toContain("response?.error ?? response?.Error");
  });
});
