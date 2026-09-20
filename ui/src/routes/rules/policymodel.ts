// The wire shapes, the labels, and the conversion between them. The policies
// page and its two child components share this module, so a field rename in
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

// One rule inside a policy group. A rule carries no domain list: the group's
// categories and domains are the coverage a rule refines.
export type GroupRule = {
  id: string;
  name: string;
  enabled: boolean;
  action: string;
  mitm_action: string;
  url_regexes: string[];
  blocked_content_types: string[];
  schedule: Schedule | null;
  users: string[];
  priority: number;
  // Entry text for the two list inputs. persistableGroup strips both fields, so
  // they never leave the browser.
  url_draft: string;
  content_type_draft: string;
};

export type PolicyGroup = {
  id: string;
  name: string;
  description: string;
  domains: string[];
  categories: string[];
  rules: GroupRule[];
  action: string;
  users: string[];
  schedule?: Schedule | null;
  unknown_device_policy: string;
  priority: number;
};

export type PolicyTemplate = {
  id: string;
  name: string;
  group_name: string;
  description: string;
  limitations: string[];
  categories: string[];
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

// A device only leaves the gateway default policy once it is assigned to a
// group, so the group list shows the assignments that make it effective.
export type DeviceAssignment = {
  device_id: string;
  group_id: string;
};

// Discovery owns the observed device record; the page only reads the labels it
// needs to name a device in an assignment control.
export type Device = {
  id: string;
  display_name?: string;
  hostnames?: string[];
  ipv4?: string;
  online?: boolean;
};

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

// "default" keeps the gateway's own HTTPS inspection setting, which is the
// setting an administrator who never touches this field expects.
export const MITM_DEFAULT = "default";

// The active-hours choice that means "the times entered in this form" rather
// than one of the named presets.
export const SCHEDULE_PRESET_CUSTOM = "custom";

export function emptyGroup(): PolicyGroup {
  return {
    id: "",
    name: "",
    description: "",
    domains: [],
    categories: [],
    rules: [],
    action: "",
    users: [],
    schedule: null,
    unknown_device_policy: "",
    priority: 0,
  };
}

export function emptyRule(): GroupRule {
  return {
    id: "",
    name: "",
    enabled: true,
    action: "block",
    mitm_action: MITM_DEFAULT,
    url_regexes: [],
    blocked_content_types: [],
    schedule: null,
    users: [],
    priority: 0,
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

function copyRule(rule: GroupRule): GroupRule {
  return {
    id: rule.id || "",
    name: rule.name || "",
    enabled: rule.enabled !== false,
    action: rule.action === "allow" ? "allow" : "block",
    mitm_action: rule.mitm_action || MITM_DEFAULT,
    url_regexes: [...(rule.url_regexes || [])],
    blocked_content_types: [...(rule.blocked_content_types || [])],
    schedule: copySchedule(rule.schedule),
    users: [...(rule.users || [])],
    priority: rule.priority || 0,
    url_draft: "",
    content_type_draft: "",
  };
}

// The editor lists rules in the order the evaluator applies them, so moving a
// rule up changes which one wins rather than only how the list looks.
function evaluationOrder(rules: GroupRule[]): GroupRule[] {
  return [...rules].sort((a, b) => {
    if ((a.priority || 0) !== (b.priority || 0)) return (a.priority || 0) - (b.priority || 0);
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
  });
}

// copyGroup turns an API record into an editable draft. Every field the form
// touches gets a value, so no control is bound to undefined.
export function copyGroup(group: PolicyGroup): PolicyGroup {
  return {
    id: group.id,
    name: group.name,
    description: group.description || "",
    domains: [...(group.domains || [])],
    categories: [...(group.categories || [])],
    rules: evaluationOrder(group.rules || []).map(copyRule),
    action: group.action || "",
    users: [...(group.users || [])],
    schedule: copySchedule(group.schedule),
    unknown_device_policy: group.unknown_device_policy || "",
    priority: group.priority || 0,
  };
}

// persistableGroup is the request body. Rule priority comes from the position
// in the list, and the list-input entry text is dropped, so the body carries
// exactly the fields the API contract names.
export function persistableGroup(draft: PolicyGroup) {
  return {
    id: draft.id,
    name: draft.name.trim(),
    description: draft.description,
    domains: draft.domains,
    categories: draft.categories,
    action: draft.action,
    users: draft.users,
    schedule: draft.schedule ?? null,
    unknown_device_policy: draft.unknown_device_policy,
    priority: draft.priority,
    rules: draft.rules.map((rule, index) => ({
      id: rule.id,
      name: rule.name.trim(),
      enabled: rule.enabled,
      action: rule.action,
      mitm_action: rule.mitm_action,
      url_regexes: rule.url_regexes,
      blocked_content_types: rule.blocked_content_types,
      schedule: rule.schedule,
      users: rule.users,
      priority: index,
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
  if (!schedule) return "Always active.";
  const windows = (schedule.windows || [])
    .map((window) => window.from + "-" + window.to)
    .join(", ");
  const days = weekdayLabel(schedule.weekdays);
  return windows + (days ? " " + days : " every day") + " (" + (schedule.timezone || "UTC") + ")";
}

// conditionLabel is the one-line read view of a rule: what narrows it beyond
// the group's own categories and domains.
export function conditionLabel(rule: GroupRule): string {
  const parts: string[] = [];
  if (rule.url_regexes?.length) {
    parts.push(rule.url_regexes.length + (rule.url_regexes.length === 1 ? " URL pattern" : " URL patterns"));
  }
  if (rule.blocked_content_types?.length) parts.push(rule.blocked_content_types.join(", "));
  if (rule.users?.length) parts.push("users " + rule.users.join(", "));
  parts.push(scheduleLabel(rule.schedule));
  return parts.join(" - ");
}
