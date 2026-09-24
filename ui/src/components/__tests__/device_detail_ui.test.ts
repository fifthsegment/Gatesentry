import { expect, test } from "vitest";
import deviceDetailSource from "../../routes/devices/devicedetail.svelte?raw";

// Prettier wraps long text nodes and attributes across lines; normalize
// whitespace so assertions on user-facing strings stay stable.
const source = deviceDetailSource.replace(/\s+/g, " ");

test("device detail assigns a policy explicitly", () => {
  expect(source).toContain('"/api/policy"');
  expect(source).toContain('POLICY_BASE + "/groups"');
  expect(source).toContain("/assignment");
  expect(source).toContain("Default policy");
  expect(source).toContain("group_id: selectedGroup");
  expect(source).toContain(
    "A device without a policy of its own uses the default policy.",
  );
});

test("device detail shows effective protection without overclaiming", () => {
  expect(source).toContain("/policy");
  expect(source).toContain("confidence");
  expect(source).toContain("coverage?.summary");
  expect(source).toContain("effective_policy");
  expect(source).toContain("coverage?.caveats");
  expect(source).toContain("so the default policy applies");
  expect(source).toContain("shared address");
  expect(source).toContain("stale observation");
});

test("device detail labels metadata as non-enforcing", () => {
  expect(source).toContain(
    "Descriptive label only — it does not change filtering.",
  );
  expect(source).toContain("metadata_note");
});

test("device detail links decision history to the log view", () => {
  expect(source).toContain("/activity");
  expect(source).toContain("View full decision history");
  expect(source).toContain('"/logs"');
  expect(source).toContain("decision history");
});

test("device detail saves labels through the modal submit event", () => {
  // Carbon's ModalFooter primary button calls the ComposedModal submit
  // context, so the handler must be on the modal, not the footer.
  expect(source).toContain("on:submit={save}");
  expect(source).not.toContain("on:click:button--primary");
});
