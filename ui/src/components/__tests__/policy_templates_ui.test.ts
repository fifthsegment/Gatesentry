import { expect, test } from "vitest";
import rulesSource from "../../routes/rules/rules.svelte?raw";
import groupsSource from "../../routes/rules/policygroups.svelte?raw";
import formSource from "../../routes/rules/groupform.svelte?raw";
import modelSource from "../../routes/rules/policymodel.ts?raw";
import categorySource from "../../routes/rules/categoryselect.svelte?raw";

const rules = rulesSource.replace(/\s+/g, " ");
const groups = groupsSource.replace(/\s+/g, " ");
const form = formSource.replace(/\s+/g, " ");
const model = modelSource.replace(/\s+/g, " ");
const category = categorySource.replace(/\s+/g, " ");

test("the policies page has one editor and no separate advanced rule list", () => {
  expect(rules).toContain("Policygroups");
  // Rules live inside a policy group, so the standalone editor is gone.
  expect(rules).not.toContain("Advanced rule editor");
  expect(rules).not.toContain("Rulelist");
});

test("a policy group owns its categories, domains, and rules", () => {
  expect(form).toContain("Categories");
  expect(form).toContain("Domains");
  expect(form).toContain("Rules");
  expect(form).toContain("Add domain");
  expect(form).toContain("Add rule");
  expect(form).toContain("Remove rule");
  expect(form).toContain("Save group");
  expect(form).toContain("draft.rules");
  // Rule order is evaluation order, so the form can move a rule rather than
  // only list it.
  expect(form).toContain("moveRule");
  expect(form).toContain("Move rule up");
  expect(form).toContain("Move rule down");
  // The body the page sends carries the rules and their order.
  expect(model).toContain("rules: draft.rules.map");
  expect(model).toContain("priority: index");
});

test("a group states what each rule adds beyond the group's own coverage", () => {
  expect(groups).toContain("group.rules");
  expect(groups).toContain("conditionLabel(rule)");
  expect(groups).toContain("No rules. The action above applies to the whole group.");
  expect(model).toContain("export function conditionLabel");
  // URL and response-type conditions need a decrypted request, so the form only
  // enables them for a block rule that keeps inspection on.
  expect(form).toContain("conditionsAllowed");
});

test("policy templates preview their categories and their own caveat", () => {
  expect(groups).toContain("templateData.templates");
  expect(groups).toContain("template.name");
  expect(model).toContain("group_name: string");
  expect(model).toContain("available: boolean");
  expect(groups).toContain("limitations");
  expect(groups).toContain("template.limitations");
  // The caveats that hold for every starter arrive once from the API, so no
  // tile repeats them.
  expect(groups).toContain("shared_limitations");
  expect(groups).toContain("templateData.shared_limitations");
  expect(groups).toContain("Applies to every starter");
  expect(groups).not.toContain("protections");
  // A starter names its categories as tags instead of restating them in prose.
  expect(groups).toContain("categoryName(category)");
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
});

test("self-updating categories are selectable gateway-wide and per group", () => {
  expect(groups).toContain("/api/policy/categories");
  expect(groups).toContain("Blocked categories");
  expect(groups).toContain("Save categories");
  expect(groups).toContain("Categoryselect");
  expect(groups).toContain("selection={gatewaySelection}");
  // The shared control reports a whole new selection, so the page never mutates
  // the array the API returned.
  expect(groups).toContain("gatewaySelection = event.detail");
  expect(form).toContain("draft.categories = event.detail");
  expect(category).toContain("category.description");
  expect(category).toContain("coverageLabel(category)");
  expect(category).toContain("Not downloaded yet");
});

test("a group covers explicit domains as well as whole categories", () => {
  expect(form).toContain("Add a domain pattern");
  expect(form).toContain("example.com or *.example.com");
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
