import { expect, test } from "vitest";
import rulesSource from "../../routes/rules/rules.svelte?raw";
import groupsSource from "../../routes/rules/policygroups.svelte?raw";

const rules = rulesSource.replace(/\\s+/g, " ");
const groups = groupsSource.replace(/\\s+/g, " ");

test("rules page keeps templates and advanced editor available", () => {
  expect(rules).toContain("Policygroups");
  expect(rules).toContain("Advanced rule editor");
  expect(rules).toContain("Rulelist");
});

test("policy templates preview supported behavior and limitations", () => {
  expect(groups).toContain("templateData.templates");
  expect(groups).toContain("template.name");
  expect(groups).toContain("group_name: string");
  expect(groups).toContain("available: boolean");
  expect(groups).toContain("protections");
  expect(groups).toContain("limitations");
  expect(groups).toContain("Review the protections and limitations first");
  expect(groups).toContain("template.protections");
  expect(groups).toContain("template.limitations");
});

test("applying templates creates ordinary editable groups without overwrite", () => {
  expect(groups).toContain("/api/policy/templates");
  expect(groups).toContain("/apply");
  expect(groups).toContain("Apply as editable group");
  expect(groups).toContain("applying it again never resets");
  expect(groups).toContain("/api/policy/groups");
  expect(groups).toContain("Create policy group");
  expect(groups).toContain("Edit");
  expect(groups).toContain("Delete");
  expect(groups).toContain("Domain pattern");
});
