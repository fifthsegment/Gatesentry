// The wire shapes, the labels, and the conversion between them. The policies
// page and its child components share this module, so a field rename in
// application/policy/types.go is a compile error here instead of a property
// that is silently dropped on the way to the API.

export type TimeWindow = { from: string; to: string };

// A schedule is evaluated in its own IANA time zone. Weekdays use Go's
// time.Weekday numbering, so Sunday is 0.
export type Schedule = {
  timezone: string;
  weekdays: number[];
  windows: TimeWindow[];
  preset?: string;
};

export type SchedulePreset = {
  id: string;
  name: string;
  description: string;
  timezone: string;
  weekdays: number[];
  windows: TimeWindow[];
};

// What a rule applies to: every request, or the listed domains and
// categories.
export type RuleTarget = {
  all_traffic: boolean;
  domains: string[];
  categories: string[];
};

// One rule inside a policy. Rules run top to bottom before the policy's
// blocked and allowed lists, and the first match decides.
export type GroupRule = {
  id: string;
  name: string;
  enabled: boolean;
  action: string;
  target: RuleTarget;
  url_regexes: string[];
  blocked_content_types: string[];
  schedule: Schedule | null;
  users: string[];
  // Entry text for the list inputs. persistableGroup strips them, so they
  // never leave the browser.
  domain_draft: string;
  url_draft: string;
  content_type_draft: string;
};

export type PolicyGroup = {
  id: string;
  name: string;
  description: string;
  blocked_categories: string[];
  blocked_domains: string[];
  allowed_domains: string[];
  rules: GroupRule[];
  safe_search: boolean;
  users: string[];
  priority: number;
};

export type PolicyTemplate = {
  id: string;
  name: string;
  group_name: string;
  description: string;
  limitations: string[];
  categories: string[];
  safe_search: boolean;
  rules?: GroupRule[];
  group_id: string;
  available: boolean;
};

// One self-updating domain feed. The counts come from the last download, so
// the page can show what a category actually covers.
export type CategoryStatus = {
  id: string;
  name: string;
  description: string;
  domain_count: number;
  updated_at?: string;
  enabled: boolean;
};

export type DeviceAssignment = {
  device_id: string;
  group_id: string;
};

// Discovery owns the observed device record; the page only reads the labels it
// needs to name a device.
export type Device = {
  id: string;
  display_name?: string;
  hostnames?: string[];
  ipv4?: string;
  online?: boolean;
};

export type TraceStep = {
  stage: string;
  applied: boolean;
  action?: string;
  detail?: string;
};

export type PreviewEvaluation = {
  action: string;
  group_id: string;
  group_name: string;
  rule_id: string;
  matched_domain: string;
  reason: string;
  safe_search: boolean;
  trace: TraceStep[];
  proxy_only: string[];
  block_url_regexes?: string[];
  block_content_types?: string[];
  url_blocked: boolean;
};

export type PreviewResult = {
  identity: { device_id?: string; auth_user?: string; source: string; explanation?: string };
  active: PreviewEvaluation;
};

// The built-in policy every unassigned device and user falls back to.
export const DEFAULT_GROUP_ID = "default";

// MultiSelect items, in Go's time.Weekday order.
export const WEEKDAYS = [
  { id: "0", text: "Sunday" },
  { id: "1", text: "Monday" },
  { id: "2", text: "Tuesday" },
  { id: "3", text: "Wednesday" },
  { id: "4", text: "Thursday" },
  { id: "5", text: "Friday" },
  { id: "6", text: "Saturday" },
];

export const WEEKDAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

// The active-hours choice that means "the times entered in this form" rather
// than one of the named presets.
export const SCHEDULE_PRESET_CUSTOM = "custom";

export function emptyGroup(): PolicyGroup {
  return {
    id: "",
    name: "",
    description: "",
    blocked_categories: [],
    blocked_domains: [],
    allowed_domains: [],
    rules: [],
    safe_search: false,
    users: [],
    priority: 0,
  };
}

export function emptyRule(): GroupRule {
  return {
    id: "",
    name: "",
    enabled: true,
    action: "block",
    target: { all_traffic: false, domains: [], categories: [] },
    url_regexes: [],
    blocked_content_types: [],
    schedule: null,
    users: [],
    domain_draft: "",
    url_draft: "",
    content_type_draft: "",
  };
}

function copySchedule(schedule: Schedule | null | undefined): Schedule | null {
  if (!schedule) return null;
  return {
    timezone: schedule.timezone || "UTC",
    weekdays: [...(schedule.weekdays || [])],
    windows: (schedule.windows || []).map((window) => ({ from: window.from, to: window.to })),
    ...(schedule.preset ? { preset: schedule.preset } : {}),
  };
}

export function copyRule(rule: Partial<GroupRule>): GroupRule {
  return {
    id: rule.id || "",
    name: rule.name || "",
    enabled: rule.enabled !== false,
    action: rule.action === "allow" ? "allow" : "block",
    target: {
      all_traffic: !!rule.target?.all_traffic,
      domains: [...(rule.target?.domains || [])],
      categories: [...(rule.target?.categories || [])],
    },
    url_regexes: [...(rule.url_regexes || [])],
    blocked_content_types: [...(rule.blocked_content_types || [])],
    schedule: copySchedule(rule.schedule),
    users: [...(rule.users || [])],
    domain_draft: "",
    url_draft: "",
    content_type_draft: "",
  };
}

// copyGroup turns an API record into an editable draft. Every field the form
// touches gets a value, so no control is bound to undefined. Rules keep the
// order the API stored, which is the order they are evaluated in.
export function copyGroup(group: Partial<PolicyGroup>): PolicyGroup {
  return {
    id: group.id || "",
    name: group.name || "",
    description: group.description || "",
    blocked_categories: [...(group.blocked_categories || [])],
    blocked_domains: [...(group.blocked_domains || [])],
    allowed_domains: [...(group.allowed_domains || [])],
    rules: (group.rules || []).map(copyRule),
    safe_search: !!group.safe_search,
    users: [...(group.users || [])],
    priority: group.priority || 0,
  };
}

// persistableGroup is the request body: the list-input entry text is dropped,
// so the body carries exactly the fields the API contract names.
export function persistableGroup(draft: PolicyGroup) {
  return {
    id: draft.id,
    name: draft.name.trim(),
    description: draft.description,
    blocked_categories: draft.blocked_categories,
    blocked_domains: draft.blocked_domains,
    allowed_domains: draft.allowed_domains,
    safe_search: draft.safe_search,
    users: draft.users,
    priority: draft.priority,
    rules: draft.rules.map((rule) => ({
      id: rule.id,
      name: rule.name.trim(),
      enabled: rule.enabled,
      action: rule.action,
      target: rule.target,
      url_regexes: rule.url_regexes,
      blocked_content_types: rule.blocked_content_types,
      schedule: rule.schedule,
      users: rule.users,
    })),
  };
}

// weekdayLabel returns the days a schedule names, or an empty string when it
// covers every day.
export function weekdayLabel(weekdays: number[] | undefined): string {
  if (!weekdays?.length || weekdays.length >= 7) return "";
  return [...weekdays]
    .sort((a, b) => a - b)
    .map((day) => WEEKDAY_NAMES[day])
    .filter(Boolean)
    .join(", ");
}

export function scheduleLabel(schedule: Schedule | null | undefined): string {
  if (!schedule) return "Always";
  const windows = (schedule.windows || []).map((window) => window.from + "-" + window.to).join(", ");
  const days = weekdayLabel(schedule.weekdays);
  return windows + (days ? " " + days : " every day");
}

// targetLabel names what a rule applies to in one phrase.
export function targetLabel(target: RuleTarget, categoryName: (id: string) => string): string {
  if (target.all_traffic) return "all traffic";
  const parts = [...target.categories.map(categoryName), ...target.domains];
  if (!parts.length) return "nothing yet";
  if (parts.length > 3) return parts.slice(0, 3).join(", ") + " and " + (parts.length - 3) + " more";
  return parts.join(", ");
}

// ruleSentence is the one-line read view of a rule, written the way an
// administrator would say it: "Block all traffic 20:00-07:00 Sun, Mon".
export function ruleSentence(rule: GroupRule, categoryName: (id: string) => string): string {
  let sentence = (rule.action === "allow" ? "Allow " : "Block ") + targetLabel(rule.target, categoryName);
  if (rule.url_regexes.length) {
    sentence += " when the URL matches " + rule.url_regexes.join(" or ");
  }
  if (rule.blocked_content_types.length) {
    sentence += (rule.url_regexes.length ? " or the response is " : " when the response is ") + rule.blocked_content_types.join(", ");
  }
  if (rule.schedule) sentence += ", " + scheduleLabel(rule.schedule);
  if (rule.users.length) sentence += ", for " + rule.users.join(", ");
  return sentence;
}

// needsProxy reports whether a rule has a condition only the proxy can see, so
// the editor can say DNS alone will not enforce it.
export function needsProxy(rule: GroupRule): boolean {
  return rule.url_regexes.length > 0 || rule.blocked_content_types.length > 0 || rule.users.length > 0;
}
