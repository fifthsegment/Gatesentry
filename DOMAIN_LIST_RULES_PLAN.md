# Domain List & Rules Enhancement Plan

## Status: PLANNING

---

## 1. Problem Statement

GateSentry currently has **two completely separate domain-blocking systems** that share no data:

| System | Entries | Scope | Per-User? |
|--------|---------|-------|-----------|
| **DNS Blocklists** | ~1.5M domains (14 remote URLs: StevenBlack, AdguardDNS, Hagezi, etc.) | DNS server only — intercepts at resolution time | ❌ Global only |
| **Proxy "Block List" filter** (`/blockedurls`) | 1 domain (`snapads.com`) in `blockedsites.json` | Proxy only — checked via `FilterUrlBlockedHosts()` | ❌ Global only |
| **Rules** (`/rules`) | Admin-created rules with single domain patterns | Proxy only — checked via `CheckProxyRules()` | ✅ Per-user, per-time, priority-based |

**Consequences:**
- Turning off DNS filtering means 1.5M blocked domains are **not enforced by the proxy** (they aren't shared)
- The proxy's "Block List" filter at `/blockedurls` is manually curated and tiny — it's not connected to the DNS blocklists
- Users can't leverage the extensive DNS blocklists within the per-user Rules system
- Content filtering (MITM) can block by Content-Type or URL regex, but **not** by embedded resource domain

---

## 2. Current Architecture (What Exists Today)

### 2.1 Proxy Filter System (`/blockedurls`, `/blockedkeywords`, etc.)

**These are PROXY-LEVEL, GLOBAL filters — not DNS.**

| Page Route | Filter ID | Filter Name | File | Purpose |
|------------|-----------|-------------|------|---------|
| `/blockedurls` | `bTXmTXgTuXpJuOZ` | Blocked URLs | `blockedsites.json` | Block entire domains (currently 1 entry) |
| `/blockedkeywords` | `bVxTPTOXiqGRbhF` | Keywords to Block | `stopwords.json` | Score-based keyword blocking in HTML |
| `/blockedfiletypes` | `JHGJiwjkGOeglsk` | Blocked content types | `blockedmimes.json` | Block by MIME type (e.g., `video/mp4`) |
| `/excludehosts` | `CeBqssmRbqXzbHR` | Exception Hosts | `dontbump.json` | Skip SSL inspection for these hosts |
| `/excludeurls` (hidden) | `JHGJiwjkGOeglsd` | Exception URLs | `exceptionsitelist.json` | Never block these URLs |

**Key characteristics:**
- Stored as JSON files on disk (via `go-bindata` embedded defaults + MapStore overrides)
- Each filter is a `[]GsFilterLine` with `Content` and `Score` fields
- Matching is `strings.Contains()` — linear scan, case-sensitive
- **Global** — applies to all users, no per-user differentiation
- Editable via admin UI using the `Filtereditor` component

### 2.2 DNS Blocklist System (`/dns` page → "DNS Block lists" section)

- Stored as a JSON array of URLs in MapStore under key `dns_custom_entries`
- Downloaded by `dns/filter/domains.go` → parsed from hosts-file format
- Loaded into `map[string]bool` in the DNS server package (in-memory only)
- Refreshed every 10 hours by the scheduler
- **Never shared** with the proxy filter system or the rules engine

### 2.3 Rules System (`/rules` page)

**Per-user, priority-based rules — PROXY-LEVEL.**

```
Rule struct:
  ID, Name, Enabled, Priority
  Domain        string        ← single pattern, supports *.example.com
  Action        "allow"|"block"
  MITMAction    "enable"|"disable"|"default"
  BlockType     "none"|"content_type"|"url_regex"|"both"
  BlockedContentTypes []string
  URLRegexPatterns    []string
  TimeRestriction     {From, To}
  Users               []string
  Description, CreatedAt, UpdatedAt
```

**Matching flow in proxy:**
1. `CheckProxyRules(host, user)` → `RuleManager.MatchRule(domain, user)`
2. Linear scan through rules sorted by priority
3. First match wins → returns `RuleMatch` with MITM/block decisions
4. Proxy uses `reflect` to read the result (cross-module boundary)

**Key limitation:** Each rule matches ONE domain pattern. No support for domain lists.

---

## 3. Proposed Design

### 3.1 New Concept: "Domain Lists"

A **Domain List** is a named, reusable collection of domains. It is the **universal building block** for all domain-based blocking in GateSentry — DNS blocking, proxy rules, and content filtering all reference Domain Lists.

Every Domain List has exactly **one source type** (mutually exclusive):
- **URL-sourced** — populated from a remote URL (hosts-file or plain-domain-list format). Periodically refreshed. Admin cannot manually add/remove individual domains. Examples: StevenBlack, Hagezi, AdguardDNS.
- **Local (admin-managed)** — the admin creates the list and manually adds/removes domains via the UI. No remote URL. Examples: "My Ad Servers", "Blocked Social Media", "Company Policy Blocks".

Each Domain List is:
- Identified by a unique ID and human-readable name
- Stored as metadata in MapStore (name, source type, description, enabled flag)
- Loaded into an **indexed in-memory structure** for O(1) lookup
- For URL-sourced: periodically refreshed from the remote URL (reusing the existing scheduler pattern)
- For local lists: domains stored persistently and loaded into the index at boot

#### What happens to `/blockedurls`?

The existing `/blockedurls` page (which edits the `blockedsites.json` proxy filter with 1 entry) is **replaced** by the Domain Lists management UI. Instead of a single flat list, the admin can:
- **Create multiple local Domain Lists** — each with its own name and purpose
- **Add/remove domains** to each local list individually
- **View URL-sourced lists** (read-only domain content, editable metadata)
- **Reference any list from Rules** for per-user, priority-based, time-restricted enforcement

The legacy `blockedsites.json` filter and its `FilterUrlBlockedHosts` handler are **retired**. Any existing entries are migrated into a local Domain List named "Blocked Sites (migrated)".

#### Domain List Storage Schema

```go
type DomainList struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`            // e.g., "StevenBlack Ads" or "My Custom Blocks"
    Description string   `json:"description"`
    Category    string   `json:"category"`         // e.g., "Ads", "Malware", "Adult", "Social Media", "Custom"
    Source      string   `json:"source"`           // "url" or "local" (mutually exclusive)
    URL         string   `json:"url,omitempty"`    // Remote URL (when source="url")
    Domains     []string `json:"domains,omitempty"` // Admin-managed entries (when source="local")
    EntryCount  int      `json:"entry_count"`      // Cached count (read-only, computed)
    LastUpdated string   `json:"last_updated"`     // ISO 8601 timestamp
    CreatedAt   string   `json:"created_at"`
}

// A Domain List has NO enabled/disabled flags. It is just a data container.
// Usage is determined by WHO REFERENCES IT:
//
// DNS blocking:
//   The DNS server config maintains two sets of Domain List IDs in MapStore:
//     "dns_domain_lists"           — blocklists (domains to block)
//     "dns_whitelist_domain_lists" — whitelists (domains to NEVER block)
//   A domain is blocked by DNS when:
//     dnsFilteringEnabled == true
//     && domain is in ANY list referenced by dns_domain_lists
//     && domain is NOT in ANY list referenced by dns_whitelist_domain_lists
//
// Proxy Rules:
//   Each Rule references Domain List IDs via its DomainLists field.
//   A domain matches a Rule when it appears in ANY referenced list.
//   The Rule's Action field ("allow"/"block") determines the outcome.
//   The Domain List itself has no knowledge of which Rules use it.

type DomainListIndex struct {
    // In-memory O(1) lookup — rebuilt on load/refresh
    // Key: domain (lowercase), Value: set of list IDs that contain it
    domains map[string]map[string]bool  // domain → {listID: true}
    mu      sync.RWMutex
}
```

#### Two Source Types — Summary

| Aspect | URL-sourced | Local (admin-managed) |
|--------|-------------|----------------------|
| **Domains come from** | Remote URL (hosts-file / plain list) | Admin adds/removes via UI |
| **Auto-refresh** | ✅ Periodic (scheduler) | ❌ N/A |
| **Admin can edit domains?** | ❌ Read-only (content from URL) | ✅ Full CRUD |
| **Admin can edit metadata?** | ✅ Name, description | ✅ Name, description |
| **Typical size** | 10K–300K domains | 1–1000 domains |
| **Category** | Auto-assigned from seed data (e.g., "Ads", "Malware") | Admin chooses on creation |
| **Examples** | StevenBlack, Hagezi, AdguardDNS | "My Blocked Ads", "Social Media" |

#### Relationship to Existing DNS Blocklists

The existing `dns_custom_entries` setting (14 URLs) will be **migrated** to become Domain Lists:
- Each URL becomes a Domain List with `source: "url"`
- The DNS server's `blockedDomains` map is replaced by the shared `DomainListIndex`
- The DNS filtering toggle (`enable_dns_filtering`) continues to control whether DNS queries are blocked
- Each migrated list's ID is added to the `dns_domain_lists` config (so DNS continues to use them)
- The `/dns` page UI for managing blocklist URLs migrates to an add/remove interface for `dns_domain_lists`

**Migration is backward-compatible:**
- On first boot after upgrade, if `dns_custom_entries` exists and no Domain Lists exist, auto-create Domain Lists from the URLs and populate `dns_domain_lists` with their IDs
- The `dns_custom_entries` setting is preserved for backward compatibility but Domain Lists become the source of truth

### 3.2 Rule Struct Expansion

```go
type Rule struct {
    // ... existing fields ...

    // Domain matching — backward compatible
    Domain         string   `json:"domain"`           // Legacy single pattern (still works)
    DomainPatterns []string `json:"domain_patterns"`   // Multiple wildcard patterns (NEW)
    DomainLists    []string `json:"domain_lists"`      // Domain List IDs to match against (NEW)

    // Content filtering — expanded
    BlockType           BlockType `json:"block_type"`
    BlockedContentTypes []string  `json:"blocked_content_types"`
    URLRegexPatterns    []string  `json:"url_regex_patterns"`
    ContentDomainLists  []string  `json:"content_domain_lists"`  // (NEW) Block embedded resources by domain list
}
```

#### Matching Logic Changes

`MatchRule()` currently checks: `matchDomain(rule.Domain, domain)`

New logic:
```
1. If rule.Domain is set → matchDomain(rule.Domain, domain) (backward compat)
2. If rule.DomainPatterns is set → any pattern matches? (wildcard check)
3. If rule.DomainLists is set → domain exists in ANY referenced Domain List? (O(1) index lookup)
4. Rule matches if ANY of the above match (OR logic)
```

#### Content Filtering Changes

Currently, MITM content filtering can block embedded resources by:
- Content-Type (e.g., block all `video/mp4`)
- URL regex (e.g., block `/ads/.*`)

**New addition:** Block embedded resources whose domain appears in a Domain List.
- Example: Allow `cnn.com` main page, but block embedded resources from domains in the "Ad Servers" list
- The proxy checks each sub-request's domain against the referenced Domain Lists

### 3.3 New Package: `application/domainlist/`

```
application/domainlist/
    manager.go       — DomainListManager: CRUD, load, refresh, index
    index.go         — DomainListIndex: O(1) domain lookup with RWMutex
    loader.go        — Download and parse remote URLs (reuse dns/filter/domains.go logic)
    migrate.go       — One-time migration from dns_custom_entries to Domain Lists
```

### 3.4 API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/domainlists` | List all Domain Lists (metadata only, no domains) |
| POST | `/api/domainlists` | Create a new Domain List |
| GET | `/api/domainlists/{id}` | Get Domain List details |
| PUT | `/api/domainlists/{id}` | Update Domain List |
| DELETE | `/api/domainlists/{id}` | Delete Domain List |
| POST | `/api/domainlists/{id}/refresh` | Force re-download of a URL-based list |
| GET | `/api/domainlists/{id}/check/{domain}` | Test if a domain is in a list |

### 3.5 UI Changes

#### Domain Lists Management Page (replaces `/blockedurls` → new: `/domainlists`)

The old `/blockedurls` page (single flat list editor for `blockedsites.json`) is **replaced** by this page.

**Main view — DataTable of all Domain Lists:**
- Columns: Name, Source (URL / Local), Entry Count, Last Updated, Enabled toggle
- "Create List" button → opens creation modal
- Row actions: Edit, Delete, Refresh (URL-sourced only)

**Create / Edit modal:**
- Name, Description, Enabled toggle
- Source type selector: "Remote URL" or "Local (manual)"
  - If URL: text field for the URL, "Test Download" button
  - If Local: domain entry panel (add/remove individual domains)

**Local list domain editing (inline or sub-page):**
- Add domain input + Add button
- Searchable/filterable domain table with delete per row
- Bulk import (paste multiple domains, one per line)
- Entry count displayed

**URL-sourced list detail view:**
- Shows URL, entry count, last refresh timestamp
- "Refresh Now" button
- Read-only domain list preview (first N entries + search)

**Test lookup:** "Check if domain X is in this list" — quick verification tool

#### Rules Form (`/rules` → `rform.svelte`)

Current "Domain" field (single TextInput) expands to:

**Domain Matching section:**
- "Domain Patterns" — tag-based input (multiple wildcards like `*.example.com`, `ads.google.com`)
- "Domain Lists" — multi-select dropdown of available Domain Lists
- Legacy `domain` field still shown if populated (backward compat)

**Content Filtering section (when MITM enabled):**
- Existing: Block by Content Type, Block by URL Pattern
- **New option:** "Block by Domain List" — multi-select of Domain Lists
- The `BlockType` enum expands to include `"domain_list"` combinations

#### DNS Page (`/dns`)

The existing "DNS Block lists" section (`dnslists.svelte`) is **replaced** with a Domain List assignment interface.

The DNS server maintains two separate configs in MapStore:
- `dns_domain_lists` — Domain List IDs used as **blocklists** (domains to block)
- `dns_whitelist_domain_lists` — Domain List IDs used as **whitelists** (domains to never block, overrides blocklists)

The `/dns` page manages both sets.

**DNS Block Lists section:**
- A DataTable of Domain Lists **currently assigned as DNS blocklists** (those in `dns_domain_lists`)
- Each row: Name, Source type (URL/Local), Category, Entry Count, Last Updated
- "Add Domain List" button → opens a picker showing all available Domain Lists not yet added
- "Remove" action per row → removes the list from DNS blocking (does NOT delete the Domain List itself)

**DNS Whitelist section:**
- A DataTable of Domain Lists **currently assigned as DNS whitelists** (those in `dns_whitelist_domain_lists`)
- Same row format and Add/Remove controls as blocklists
- Whitelisted domains override blocklists — if a domain appears in both, it is **allowed**
- This replaces the old `/excludeurls` filter (`exceptionsitelist.json`) for DNS exceptions

**Evaluation order for DNS:**
1. Is `dnsFilteringEnabled` true? If no → allow all
2. Is domain in any **whitelist**? If yes → allow (skip blocklist check)
3. Is domain in any **blocklist**? If yes → block
4. Otherwise → allow

**Seeded / migrated state:**
- The 14 existing DNS blocklist URLs migrate to URL-sourced Domain Lists, added to `dns_domain_lists`
- Entries from `exceptionsitelist.json` are migrated to a local Domain List named "DNS Exceptions (migrated)", added to `dns_whitelist_domain_lists`
- Admin can add any Domain List (URL-sourced or local) to either the blocklist or whitelist set

**No more URL management on `/dns`:**
- Creating new Domain Lists (URL-sourced or local) is done on `/domainlists`
- The DNS page only controls **which existing Domain Lists are assigned as blocklists or whitelists**

#### Navigation (`menu.ts`)

- Add "Domain Lists" under the Filters menu or as a top-level item

---

## 4. Performance Considerations

### 4.1 Domain List Index

With ~1.5M domains across 14+ lists, O(1) lookup is critical.

**Structure:** `map[string]bool` per list, wrapped in a `DomainListIndex` with:
- `sync.RWMutex` for concurrent access (reads don't block each other)
- Domain normalization: lowercase, strip trailing dot
- Rebuild index on refresh (swap pointer, not mutate in place)

**Memory estimate:**
- 1.5M domains × ~25 bytes average domain length × 2 (map overhead) ≈ **~75MB**
- This is the same memory already used by the DNS `blockedDomains` map
- With the shared index, we **eliminate** the duplicate copy (DNS + proxy would share one)

### 4.2 Rule Matching Performance

Current: Linear scan through rules, `matchDomain()` per rule.

With Domain Lists, each rule with `DomainLists` does an O(1) map lookup **per referenced list**. For a single list, this is faster than wildcard matching. However, the cost scales with the **total number of rules × lists per rule**.

**Fan-out concern:**
- Each user (or user group) may have multiple rules
- Each rule may reference multiple Domain Lists
- On every proxied request: iterate all rules by priority → for each rule, check each referenced Domain List → O(1) per list, but the multiplier matters
- Example: 10 rules × 5 lists each = 50 map lookups per request (still fast, but not free)

**Mitigations:**
- First-match-wins means most requests stop early (priority-ordered scan)
- User filtering narrows the candidate rule set before domain matching begins
- Map lookups are O(1) with no allocations — even 50–100 lookups per request is sub-microsecond
- Show list count + rule count in the admin UI so the admin understands the cost of adding more
- Future optimization if needed: pre-build a merged index for rules that share the same lists

For rules with `DomainPatterns` (wildcards), the existing `matchDomain()` logic applies — linear through the patterns array, but these are admin-curated (small count).

### 4.3 Proxy Hot Path

The proxy calls `CheckProxyRules()` on every request. The new flow:
1. Filter rules by user match (reduces candidate set)
2. Iterate remaining rules by priority
3. For each rule, check domain patterns (fast) and domain list membership (O(1) per list)
4. First match wins (same as today)

**Acceptable performance** — each individual lookup is O(1), and the total work per request is bounded by (candidate rules × lists per rule). The admin should be aware that more rules and more lists per rule increases the per-request cost, but in practice this remains sub-millisecond.

---

## 5. Implementation Phases

### Phase 1: Domain List Manager (Foundation)
**Goal:** New shared package for domain list management (URL-sourced + local), index, and API. Retire `/blockedurls` filter.

Files to create:
- `application/domainlist/manager.go` — DomainListManager with CRUD for both source types
- `application/domainlist/index.go` — O(1) lookup index
- `application/domainlist/loader.go` — URL download and hosts-file parsing
- `application/domainlist/migrate.go` — Auto-migrate `dns_custom_entries` → URL-sourced lists, `blockedsites.json` → local list
- `application/webserver/endpoints/handler_domainlists.go` — REST API handlers (including domain CRUD for local lists)

Files to modify:
- `application/webserver/webserver.go` — Register new API routes, remove `/blockedurls` filter route
- `application/runtime.go` — Initialize DomainListManager at boot
- `application/filters/loader.go` — Remove `url/all_blocked_urls` filter registration
- `ui/src/menu.ts` — Replace "Block List" nav item with "Domain Lists"

Files to retire (no longer used after migration):
- `application/filters/filter-url-blockedhosts.go` — `FilterUrlBlockedHosts` handler removed from proxy pipeline
- `filterfiles/blockedsites.json` — Entries migrated to a local Domain List

UI to create:
- Domain Lists management page with:
  - List creation (URL-sourced or local)
  - Local list domain editor (add/remove/bulk-import domains)
  - URL-sourced list detail view (read-only domains, refresh button)
  - Test lookup tool

API additions for local list domain management:

| Method | Path | Purpose |
|--------|------|--------|
| GET | `/api/domainlists/{id}/domains` | List domains in a local list (paginated) |
| POST | `/api/domainlists/{id}/domains` | Add domain(s) to a local list |
| DELETE | `/api/domainlists/{id}/domains/{domain}` | Remove a domain from a local list |

### Phase 2: DNS Server Migration
**Goal:** DNS server uses the shared DomainListIndex instead of its own `blockedDomains` map.

Files to modify:
- `application/dns/server/server.go` — Replace `blockedDomains` lookups with DomainListIndex
- `application/dns/filter/domains.go` — Delegate to DomainListManager for downloads
- `application/dns/scheduler/scheduler.go` — Trigger DomainListManager refresh instead

### Phase 3: Rule Struct Expansion + Domain Matching
**Goal:** Rules can reference Domain Patterns (plural) and Domain Lists.

Files to modify:
- `application/types/rule.go` — Add `DomainPatterns`, `DomainLists` fields
- `application/rules.go` — Update `MatchRule()` to check patterns array + domain list index
- `ui/src/routes/rules/rform.svelte` — Expand domain section in the form

### Phase 4: Content Filtering by Domain List
**Goal:** MITM content filtering can block embedded resources by domain list membership.

Files to modify:
- `application/types/rule.go` — Add `ContentDomainLists` field, expand `BlockType`
- `application/rules.go` — Add `CheckContentDomainBlocked()` function
- `gatesentryproxy/proxy.go` — Check embedded resource domains against lists
- `ui/src/routes/rules/rform.svelte` — Add domain list selection to content filtering section

---

## 6. Clarification: Filters vs. Rules

| Aspect | Filters (`/blockedurls`, etc.) | Rules (`/rules`) |
|--------|-------------------------------|-------------------|
| **Scope** | **Global** — applies to ALL users | **Per-user** — can target specific users |
| **System** | **Proxy only** (not DNS) | **Proxy only** (not DNS) |
| **Matching** | `strings.Contains()` — substring match | `matchDomain()` — exact or `*.wildcard` |
| **Priority** | All filters run, any match blocks | Priority-ordered, first match wins |
| **Time-based** | ❌ | ✅ Time restrictions |
| **MITM control** | ❌ | ✅ Per-rule SSL inspection |
| **Content filtering** | ❌ (separate filter for MIME types) | ✅ Content-Type + URL regex + (NEW: domain list) |
| **Storage** | JSON files (`blockedsites.json`, etc.) | JSON blob in MapStore under `"rules"` key |
| **UI** | Simple add/remove list editor | Full form with all rule fields |

**The Filters at `/blockedurls` are global proxy-level URL blockers.** They are NOT DNS-related. They check every proxied request's URL against the manually curated `blockedsites.json` file (currently containing 1 entry: `snapads.com`).

**The Rules at `/rules` are the advanced, per-user system** that supports domain matching, MITM control, content filtering, time restrictions, and user targeting. This is where Domain Lists will be integrated.

**The DNS Blocklists at `/dns` are DNS-level only** — they populate the `blockedDomains` map used during DNS resolution. They are not checked by the proxy at all today.

---

## 7. Data Flow After Implementation

```
                         ┌──────────────────────────────────────┐
                         │       DomainListManager              │
                         │    (application/domainlist/)         │
                         │                                      │
                         │  URL-sourced lists:                  │
                         │   - StevenBlack Ads (URL)            │
                         │   - Hagezi Threats (URL)             │
                         │   - AdguardDNS (URL)                 │
                         │   - ... (14+ remote lists)           │
                         │                                      │
                         │  Local (admin-managed) lists:        │
                         │   - My Blocked Ads (local)           │
                         │   - Social Media Block (local)       │
                         │   - Company Policy (local)           │
                         │   - Blocked Sites [migrated] (local) │
                         │                                      │
                         │  DomainListIndex:                    │
                         │   map[domain] → {listIDs}            │
                         │   O(1) lookup, RWMutex               │
                         └──────────┬───────────────────────────┘
                                    │
                         ┌──────────┴──────────┐
                         │                     │
                  ┌──────▼──────┐       ┌──────▼────────┐
                  │ DNS Server  │       │ Rule Engine   │
                  │             │       │               │
                  │ Config:     │       │ MatchRule()   │
                  │ dns_domain_ │       │ checks:       │
                  │ lists =     │       │ 1. Patterns   │
                  │ [id1, id2]  │       │ 2. Lists ●    │
                  │ dns_white   │       │ 3. Users      │
                  │ list =      │       │ 4. Time       │
                  │ [id5]       │       │               │
                  │             │       │ Rule.Domain   │
                  │ if master   │       │ Lists =       │
                  │ enabled:    │       │ [id3, id4]    │
                  │ whitelist?  │       │               │
                  │  → allow    │       │ Per-user,     │
                  │ blocklist?  │       │ per-rule list │
                  │  → block    │       │               │
                  └─────────────┘       └───────────────┘
```

---

## 8. Open Questions

1. ~~Should the existing Filters (`/blockedurls`) also support Domain Lists?~~

   **RESOLVED:** No — `/blockedurls` does not "support" Domain Lists. It is **replaced entirely** by the Domain Lists management page. Everything is a Domain List. The concept of a separate "Blocked URLs filter" (`blockedsites.json` + `FilterUrlBlockedHosts`) is retired.

   **What changes:**
   - The `/blockedurls` route becomes `/domainlists` (or the menu item redirects)
   - `blockedsites.json` entries are migrated to a local Domain List named "Blocked Sites (migrated)"
   - `FilterUrlBlockedHosts` handler is removed from the proxy filter chain
   - The admin creates **local Domain Lists** (as many as needed) and adds/removes domains manually
   - URL-sourced lists (the 14 DNS blocklists) are also shown here with read-only domain content
   - Any Domain List can be referenced by Rules for per-user enforcement
   - The remaining 3 proxy filters (`blockedkeywords`, `blockedfiletypes`, `excludehosts`) are **unaffected** — they continue to work as global proxy filters
   - `/excludeurls` (`exceptionsitelist.json`) is also retired — migrated to a whitelist Domain List (see Q6)

2. ~~Should Domain Lists be visible on the `/dns` page?~~

   **RESOLVED:** Yes. The `/dns` page shows the Domain Lists **assigned to DNS filtering** via the `dns_domain_lists` config. This replaces the current URL management UI (`dnslists.svelte`).

   **Key design decisions:**
   - Domain Lists have **no enabled/disabled flags**. They are pure data containers.
   - The DNS server maintains a separate config (`dns_domain_lists` in MapStore) — a set of Domain List IDs assigned to DNS filtering.
   - The `/dns` page shows lists currently in `dns_domain_lists`, with Add/Remove controls.
   - A domain is blocked by DNS when: `dnsFilteringEnabled == true` AND domain is in ANY list referenced by `dns_domain_lists`.
   - Rules reference Domain Lists independently via `Rule.DomainLists` — no overlap with the DNS config.
   - The 14 existing DNS blocklist URLs are migrated as URL-sourced Domain Lists and their IDs are added to `dns_domain_lists`.
   - Creating new Domain Lists is done on `/domainlists`; the DNS page only assigns existing lists.

3. ~~Domain List refresh schedule~~

   **RESOLVED:** Keep the existing 10-hour scheduler interval. It applies to **URL-sourced Domain Lists only** (local lists have no remote source to refresh). No per-list configurable intervals needed.

4. ~~Maximum Domain List size~~

   **RESOLVED:** No hard limit on Domain List size. The `map[string]bool` approach handles 1.5M+ domains (~75MB RAM). Show entry count and total memory consumption in the Domain Lists UI so the admin has visibility.

   **Performance note:** The real scaling concern is not list size (lookups are O(1)) but **rule fan-out** — more rules referencing more lists means more lookups per proxied request. This is documented in Section 4.2. The UI should help admins understand the cost (show rule count, lists-per-rule, total lists assigned to DNS).

5. ~~Domain List sharing between DNS and proxy~~

   **RESOLVED:** Yes — single `DomainListManager` instance, single source of truth, no data duplication.

   - DNS checks `dns_domain_lists` config → looks up domain in referenced lists
   - Proxy Rules check `Rule.DomainLists` → looks up domain in referenced lists
   - Both use the same underlying `DomainListIndex`

   **UI hint:** When selecting Domain Lists in the Rules form, lists that are already assigned to DNS filtering (`dns_domain_lists`) show a **tag bubble** (e.g., "DNS Filtered") so the admin knows the list is already active at the DNS level. However, the admin is **not prevented** from also adding it to a proxy rule — they may disable DNS filtering in the future and want proxy-level enforcement as a fallback.

6. **Should Domain Lists support whitelists?**

   **RESOLVED:** Yes. A Domain List is a pure data container — its purpose (block or allow) is determined by **where it's assigned**, not by a field on the list itself.

   **DNS whitelisting:**
   - The DNS server config gets a second set: `dns_whitelist_domain_lists` (alongside `dns_domain_lists`)
   - If a domain appears in a whitelisted list, it is **allowed** even if it also appears in a blocklist
   - Evaluation order: whitelist wins over blocklist (check whitelist first)
   - The `/dns` page shows two sections: "Block Lists" and "Allow Lists", each with Add/Remove

   **Proxy Rules whitelisting:**
   - Already supported — a Rule with `Action: "allow"` and `DomainLists` referencing a list acts as a whitelist
   - Higher-priority allow rules override lower-priority block rules (existing first-match-wins behavior)
   - No structural changes needed — the Rule system already handles this

   **What gets retired:**
   - `/excludeurls` (`exceptionsitelist.json` — "Never block these URLs") → migrated to a local Domain List named "DNS Exceptions (migrated)", assigned to `dns_whitelist_domain_lists`

   **What stays:**
   - `/excludehosts` (`dontbump.json` — "Skip SSL inspection") → this is about **MITM bypass**, not domain blocking. It remains as a separate proxy filter. It could become a Domain List in the future, but it serves a different purpose (SSL inspection control, not allow/block).
