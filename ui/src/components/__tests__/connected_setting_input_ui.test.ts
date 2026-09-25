import { describe, expect, test } from "vitest";
import source from "../connectedSettingInput.svelte?raw";

describe("ConnectedSettingInput", () => {
  test("supports typed custom radios while retaining boolean defaults", () => {
    expect(source).toContain("type RadioOption");
    expect(source).toContain("export let radioOptions: RadioOption[]");
    expect(source).toContain(
      'export let orientation: "horizontal" | "vertical"',
    );
    expect(source).toContain('value="true"');
    expect(source).toContain('value="false"');
  });

  test("shows save progress and failures and calls onSaved after success", () => {
    expect(source).toContain("let saving = false");
    expect(source).toContain("Setting not saved");
    expect(source).toContain("data = persistedValue");
    expect(source).toContain("<InlineLoading");
    expect(source).toContain("await onSaved(keyValue)");
    expect(source.indexOf("persistedValue = keyValue")).toBeLessThan(
      source.indexOf("await onSaved(keyValue)"),
    );
  });
});
