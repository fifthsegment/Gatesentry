import { expect, test } from "vitest";
import rulesSource from "../../routes/rules/rules.svelte?raw";
import groupsSource from "../../routes/rules/policygroups.svelte?raw";
import formSource from "../../routes/rules/groupform.svelte?raw";
import modelSource from "../../routes/rules/policymodel.ts?raw";
import categorySource from "../../routes/rules/categoryselect.svelte?raw";
import domainSource from "../../routes/rules/domainlist.svelte?raw";
import {
  copyGroup,
  emptyRule,
  needsProxy,
  persistableGroup,
  ruleSentence,
} from "../../routes/rules/policymodel";

const rules = rulesSource.replace(/\s+/g, " ");
const groups = groupsSource.replace(/\s+/g, " ");
const form = formSource.replace(/\s+/g, " ");
const model = modelSource.replace(/\s+/g, " ");
const category = categorySource.replace(/\s+/g, " ");
const domains = domainSource.replace(/\s+/g, " ");

test("the policies page has one editor and route-local Carbon tabs", () => {
  expect(rules).toContain("Policygroups");
  expect(rules).not.toContain("Advanced rule editor");
  expect(rules).not.toContain("Rulelist");
  expect(groups).toContain('<Tab label="Policies"');
  expect(groups).toContain('<Tab label="Test a site"');
  expect(groups).toContain('<Tab label="Gateway defaults"');
  expect(groups).toContain("<TabContent>");
  expect(groups).not.toContain('<details class="section fold"');
});

test("a policy has blocked and always-allowed lists, safe search, and rules", () => {
  expect(form).toContain("Blocked categories");
  expect(form).toContain("Blocked domains");
  expect(form).toContain("Always allowed domains");
  expect(form).toContain("draft.blocked_categories");
  expect(form).toContain("draft.blocked_domains");
  expect(form).toContain("draft.allowed_domains");
  expect(form).toContain("Safe search");
  expect(form).toContain("draft.safe_search");
  expect(form).toContain("Add rule");
  expect(form).toContain("Remove rule");
  expect(form).toContain("Save policy");
  expect(form).toContain('legendText="Basic protection"');
  expect(form).toContain('legendText="Schedule"');
  expect(form).toContain('legendText="Advanced rules"');
  expect(form).toContain("<AccordionItem");
  expect(form).toContain("position: sticky");
  expect(form).toContain('aria-label="Policy actions"');
  // Rule order is evaluation order, so the form can move a rule.
  expect(form).toContain("Move rule up");
  expect(form).toContain("Move rule down");
});

test("a rule names its own target, including all traffic", () => {
  expect(form).toContain("Applies to");
  expect(form).toContain("All traffic");
  expect(form).toContain("rule.target.categories");
  expect(form).toContain("rule.target.domains");
  // URL and response-type conditions only narrow a block rule, and the form
  // says when a rule is enforced by the proxy only.
  expect(form).toContain("conditionsAllowed");
  expect(form).toContain("Block only part of the site");
  expect(form).toContain("enforced by the proxy only");
});

test("every device is on one policy and the page manages assignment", () => {
  expect(groups).toContain("DEFAULT_GROUP_ID");
  expect(groups).toContain("Assigned devices");
  expect(groups).toContain("setDevicePolicy");
  expect(groups).toContain("assignSelectedDevices");
  expect(groups).toContain('"/assignment"');
  expect(groups).toContain("Every device follows one policy");
  expect(groups).toContain('titleText="Add devices"');
  // The default policy cannot be deleted.
  expect(groups).toContain("group.id !== DEFAULT_GROUP_ID");
});

test("the site tester runs the live evaluator", () => {
  expect(groups).toContain("Test a site");
  expect(groups).toContain("/api/policy/preview");
  expect(groups).toContain("testResult.active.trace");
  expect(groups).toContain("testResult.active.proxy_only");
});

test("starter policies are the first step of create", () => {
  expect(groups).toContain("templateData.templates");
  expect(groups).toContain("template.limitations");
  expect(groups).toContain("templateData.shared_limitations");
  expect(groups).toContain("Starter limitations");
  expect(groups).toContain("categoryName(category)");
  expect(groups).toContain("openCreate");
  expect(groups).toContain('editingGroupId === "choose"');
  expect(groups).toContain("Use starter");
  expect(groups).toContain("Start blank");
  expect(groups).toContain("/apply");
});

test("policy deletion uses an accessible Carbon confirmation", () => {
  expect(groups).toContain("<ComposedModal");
  expect(groups).toContain('title="Delete policy?"');
  expect(groups).toContain("on:submit={confirmDeleteGroup}");
  expect(groups).toContain("primaryButtonDisabled={saving}");
  expect(groups).not.toContain("confirm(");
});

test("gateway-wide categories stay selectable", () => {
  expect(groups).toContain("/api/policy/categories");
  expect(groups).toContain("Save categories");
  expect(groups).toContain("selection={gatewaySelection}");
  expect(category).toContain("coverageLabel(category)");
  expect(category).toContain("Not downloaded yet");
  expect(category).toContain("compact");
});

test("a domain entry accepts a pasted URL", () => {
  expect(domains).toContain("covers its subdomains");
  expect(domains).toContain('indexOf("://")');
});

test("the model round-trips a policy into the API body", () => {
  const group = copyGroup({
    id: "kids",
    name: " Kids ",
    blocked_domains: ["tiktok.com"],
    rules: [
      {
        ...emptyRule(),
        id: "bed",
        name: "Bedtime",
        target: { all_traffic: true, domains: [], categories: [] },
        schedule: {
          timezone: "UTC",
          weekdays: [0, 1],
          windows: [{ from: "20:00", to: "07:00" }],
        },
      },
    ],
  });
  const body = persistableGroup(group);
  expect(body.name).toBe("Kids");
  expect(body.blocked_domains).toEqual(["tiktok.com"]);
  expect(body.rules[0].target.all_traffic).toBe(true);
  // Entry text for the list inputs never leaves the browser.
  expect(body.rules[0]).not.toHaveProperty("domain_draft");
  expect(body.rules[0]).not.toHaveProperty("url_draft");
  expect(model).toContain("export function ruleSentence");
});

test("a rule reads as a sentence and flags proxy-only conditions", () => {
  const name = (id: string) => ({ social: "Social media" })[id] || id;
  const bedtime = {
    ...emptyRule(),
    target: { all_traffic: true, domains: [], categories: [] },
    schedule: {
      timezone: "UTC",
      weekdays: [0, 1],
      windows: [{ from: "20:00", to: "07:00" }],
    },
  };
  expect(ruleSentence(bedtime, name)).toBe(
    "Block all traffic, 20:00-07:00 Sun, Mon",
  );
  expect(needsProxy(bedtime)).toBe(false);

  const shorts = {
    ...emptyRule(),
    target: {
      all_traffic: false,
      domains: ["youtube.com"],
      categories: ["social"],
    },
    url_regexes: ["/shorts/"],
  };
  expect(ruleSentence(shorts, name)).toBe(
    "Block Social media, youtube.com when the URL matches /shorts/",
  );
  expect(needsProxy(shorts)).toBe(true);
});
