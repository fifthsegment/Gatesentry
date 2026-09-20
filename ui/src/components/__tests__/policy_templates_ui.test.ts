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
  expect(groups).toContain("template.protections");
  expect(groups).toContain("template.limitations");
  // Every starter ships the same DNS-scope caveat, so it is stated once above
  // the grid while each tile keeps only the limitations it owns.
  expect(groups).toContain("Applies to every starter");
  expect(groups).toContain("sharedLimitations");
  expect(groups).toContain("ownLimitations(template)");
});

test("applying templates creates ordinary editable groups without overwrite", () => {
  expect(groups).toContain("/api/policy/templates");
  expect(groups).toContain("/apply");
  expect(groups).toContain("Apply as editable group");
  expect(groups).toContain("Applying a starter creates an ordinary editable group");
  expect(groups).toContain("never resets a group you have customized");
  expect(groups).toContain("/api/policy/groups");
  expect(groups).toContain("Create policy group");
  expect(groups).toContain("Edit");
  expect(groups).toContain("Delete");
  expect(groups).toContain("Domain pattern");
});

test("self-updating categories are selectable gateway-wide and per group", () => {
  expect(groups).toContain("/api/policy/categories");
  expect(groups).toContain("Blocked categories");
  expect(groups).toContain("category.description");
  expect(groups).toContain("coverageLabel(category)");
  expect(groups).toContain("setGatewayCategory");
  expect(groups).toContain("setDraftCategory");
  expect(groups).toContain("categories: draft.categories");
});

test("policy groups report the assignment that makes them effective", () => {
  expect(groups).toContain("/api/policy/assignments");
  expect(groups).toContain("assignmentLabel(assignedByGroup[group.id] || [])");
  expect(groups).toContain("applies only to the devices you assign to it");
});

test("devices are assigned to a group from the policy page itself", () => {
  expect(groups).toContain("/api/devices");
  expect(groups).toContain("deviceData.devices");
  // Svelte only re-renders an expression that names the state it reads, so the
  // assignments reach the template through a reactive map.
  expect(groups).toContain("$: assignedByGroup = assignmentsByGroup(assignments)");
  expect(groups).toContain("assignedByGroup[group.id]");
  expect(groups).toContain("assignableDevices(assignedByGroup[group.id] || [])");
  expect(groups).toContain("deviceName(deviceID)");
  // Each change is written through the transactional per-device endpoint rather
  // than the replace-all assignments document.
  expect(groups).toContain('DEVICES_API + "/" + deviceID + "/assignment"');
  expect(groups).toContain('method: "PUT"');
  expect(groups).toContain("JSON.stringify({ group_id: groupID })");
  expect(groups).toContain('writeAssignment([deviceID], "")');
});
