<script lang="ts">
  import {
    Button,
    InlineLoading,
    InlineNotification,
    Toggle,
  } from "carbon-components-svelte";
  import {
    Add,
    ChevronRight,
    DragVertical,
    Information,
  } from "carbon-icons-svelte";
  import { onMount, createEventDispatcher } from "svelte";
  import { getBasePath } from "../../lib/navigate";
  import { notificationstore } from "../../store/notifications";

  const dispatch = createEventDispatcher();

  let rules = [];
  let initialLoading = true;
  let saving = false;
  let error = "";

  // Drag state
  let dragIdx = -1;
  let dragOverIdx = -1;

  const API_BASE = getBasePath() + "/api/rules";

  function ruleKey(rule) {
    return rule.id || `_new_${rule.name}_${rule.priority}`;
  }

  async function loadRules() {
    error = "";
    try {
      const token = localStorage.getItem("jwt");
      if (!token) throw new Error("Please login first to view rules");
      const response = await fetch(API_BASE, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (response.status === 401)
        throw new Error("Authentication failed. Please login again");
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(
          `Failed to load rules: ${response.status} - ${errorText}`,
        );
      }
      const data = await response.json();
      rules = (data.rules || [])
        .slice()
        .sort((a, b) => a.priority - b.priority);
    } catch (err) {
      error = err.message;
    } finally {
      initialLoading = false;
    }
  }

  onMount(loadRules);

  function openRule(rule) {
    dispatch("open", rule);
  }

  function addRule() {
    const maxPriority = rules.reduce((max, r) => Math.max(max, r.priority), 0);
    const newRule = {
      id: "",
      name: "",
      enabled: true,
      priority: maxPriority + 1,
      domain: "",
      action: "allow",
      mitm_action: "default",
      blocked_content_types: [],
      url_regex_patterns: [],
      domain_patterns: [],
      domain_lists: [],
      content_domain_lists: [],
      keyword_filter_enabled: false,
      time_restriction: { from: "00:00", to: "23:59" },
      users: [],
      description: "",
    };
    dispatch("create", newRule);
  }

  // --- Mouse Drag (desktop) ---
  let dragAllowed = false;

  function allowDrag() {
    dragAllowed = true;
  }
  function disallowDrag() {
    dragAllowed = false;
  }

  function handleDragStart(e, idx) {
    if (!dragAllowed) {
      e.preventDefault();
      return;
    }
    dragIdx = idx;
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", String(idx));
    requestAnimationFrame(() => {
      rules = rules;
    });
  }

  // --- Touch Drag (mobile) ---
  let rowEls = [];
  let touchDragging = false;
  let touchClone = null;
  let touchOffsetY = 0;

  function handleTouchDragStart(e, idx) {
    e.preventDefault(); // prevent scroll while dragging from handle
    const touch = e.touches[0];
    dragIdx = idx;
    touchDragging = true;

    // Create a floating clone of the row
    const rowEl = rowEls[idx];
    if (rowEl) {
      const rect = rowEl.getBoundingClientRect();
      touchOffsetY = touch.clientY - rect.top;
      touchClone = rowEl.cloneNode(true);
      touchClone.style.position = "fixed";
      touchClone.style.left = rect.left + "px";
      touchClone.style.top = touch.clientY - touchOffsetY + "px";
      touchClone.style.width = rect.width + "px";
      touchClone.style.zIndex = "9999";
      touchClone.style.opacity = "0.85";
      touchClone.style.boxShadow = "0 4px 12px rgba(0,0,0,0.2)";
      touchClone.style.pointerEvents = "none";
      touchClone.style.transition = "none";
      document.body.appendChild(touchClone);
    }
    rules = rules; // trigger reactivity for rl-dragging class
  }

  function handleTouchDragMove(e) {
    if (!touchDragging || dragIdx === -1) return;
    e.preventDefault();
    const touch = e.touches[0];

    // Move clone
    if (touchClone) {
      touchClone.style.top = touch.clientY - touchOffsetY + "px";
    }

    // Determine which row we're over
    let overIdx = -1;
    for (let i = 0; i < rowEls.length; i++) {
      if (i === dragIdx || !rowEls[i]) continue;
      const rect = rowEls[i].getBoundingClientRect();
      if (touch.clientY >= rect.top && touch.clientY <= rect.bottom) {
        overIdx = i;
        break;
      }
    }
    dragOverIdx = overIdx;
  }

  function handleTouchDragEnd(e) {
    if (!touchDragging) return;
    // Remove clone
    if (touchClone) {
      touchClone.remove();
      touchClone = null;
    }

    if (dragIdx !== -1 && dragOverIdx !== -1 && dragIdx !== dragOverIdx) {
      const reordered = rules.slice();
      const [moved] = reordered.splice(dragIdx, 1);
      reordered.splice(dragOverIdx, 0, moved);
      const changed = [];
      for (let i = 0; i < reordered.length; i++) {
        const np = i + 1;
        if (reordered[i].priority !== np) {
          reordered[i].priority = np;
          changed.push(reordered[i]);
        }
      }
      rules = reordered;
      if (changed.length > 0) saveReorderedRules(changed);
    }
    touchDragging = false;
    cleanupDrag();
  }

  // Chevron touch-scroll protection: only fire open if finger didn't move
  let chevronTouchStartY = 0;
  let chevronTouchStartX = 0;
  const TOUCH_SLOP = 10;

  function chevronTouchStart(e) {
    const t = e.touches[0];
    chevronTouchStartX = t.clientX;
    chevronTouchStartY = t.clientY;
  }

  function chevronTouchEnd(e, rule) {
    const t = e.changedTouches[0];
    const dx = Math.abs(t.clientX - chevronTouchStartX);
    const dy = Math.abs(t.clientY - chevronTouchStartY);
    if (dx < TOUCH_SLOP && dy < TOUCH_SLOP) {
      openRule(rule);
    }
  }

  function handleDragOver(e, idx) {
    e.preventDefault();
    if (idx !== dragIdx) dragOverIdx = idx;
  }

  function handleDrop(e, idx) {
    e.preventDefault();
    if (dragIdx === -1 || dragIdx === idx) {
      cleanupDrag();
      return;
    }
    const reordered = rules.slice();
    const [moved] = reordered.splice(dragIdx, 1);
    reordered.splice(idx, 0, moved);
    const changed = [];
    for (let i = 0; i < reordered.length; i++) {
      const np = i + 1;
      if (reordered[i].priority !== np) {
        reordered[i].priority = np;
        changed.push(reordered[i]);
      }
    }
    rules = reordered;
    cleanupDrag();
    if (changed.length > 0) saveReorderedRules(changed);
  }

  function handleDragEnd() {
    cleanupDrag();
  }

  function cleanupDrag() {
    dragIdx = -1;
    dragOverIdx = -1;
    dragAllowed = false;
  }

  async function saveReorderedRules(changedRules) {
    saving = true;
    try {
      const token = localStorage.getItem("jwt");
      for (const rule of changedRules) {
        if (!rule.id) continue;
        const response = await fetch(`${API_BASE}/${rule.id}`, {
          method: "PUT",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(rule),
        });
        if (!response.ok) {
          const errorData = await response.text();
          throw new Error(`Failed to save rule priority: ${errorData}`);
        }
      }
      notificationstore.add({
        title: "Reordered",
        subtitle: `Updated priority for ${changedRules.length} rule${
          changedRules.length > 1 ? "s" : ""
        }`,
        kind: "success",
        timeout: 3000,
      });
    } catch (err) {
      notificationstore.add({
        title: "Error",
        subtitle: err.message,
        kind: "error",
        timeout: 5000,
      });
      await loadRules();
    } finally {
      saving = false;
    }
  }

  async function toggleEnabled(rule, idx) {
    rule.enabled = !rule.enabled;
    rules = rules; // reactivity
    if (!rule.id) return;
    try {
      const token = localStorage.getItem("jwt");
      const response = await fetch(`${API_BASE}/${rule.id}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(rule),
      });
      if (!response.ok) throw new Error(await response.text());
    } catch (err) {
      rule.enabled = !rule.enabled; // revert
      rules = rules;
      notificationstore.add({
        title: "Error",
        subtitle: err.message,
        kind: "error",
        timeout: 5000,
      });
    }
  }

  function actionLabel(action) {
    return action === "block" ? "Block" : "Allow";
  }

  function usersSummary(users) {
    if (!users || users.length === 0) return "Everyone";
    if (users.length <= 2) return users.join(", ");
    return `${users[0]}, +${users.length - 1}`;
  }
</script>

{#if error}
  <InlineNotification
    kind="error"
    title="Error"
    subtitle={error}
    on:close={() => (error = "")}
  />
{/if}

<div class="rl-topbar">
  <span class="rl-info"
    ><Information size={16} />All rules are evaluated in sequence</span
  >
  <Button on:click={addRule} icon={Add} size="small" kind="tertiary"
    >Add Rule</Button
  >
</div>

{#if initialLoading}
  <InlineLoading description="Loading rules..." />
{:else if rules.length === 0}
  <div class="gs-card">
    <p class="gs-empty" style="text-align: center; padding: 24px 0;">
      No rules configured. Click <strong>Add Rule</strong> to create one.
    </p>
  </div>
{:else}
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="rl-list"
    on:touchmove|nonpassive={handleTouchDragMove}
    on:touchend={handleTouchDragEnd}
    on:touchcancel={handleTouchDragEnd}
  >
    {#each rules as rule, idx (ruleKey(rule))}
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div
        class="rl-row"
        class:rl-dragging={dragIdx === idx}
        class:rl-drag-over={dragOverIdx === idx}
        class:rl-disabled={!rule.enabled}
        bind:this={rowEls[idx]}
        draggable="true"
        on:mousedown={disallowDrag}
        on:dragstart={(e) => handleDragStart(e, idx)}
        on:dragover={(e) => handleDragOver(e, idx)}
        on:dragenter|preventDefault
        on:drop={(e) => handleDrop(e, idx)}
        on:dragend={handleDragEnd}
      >
        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <span
          class="rl-drag"
          title="Drag to reorder"
          on:mousedown|stopPropagation={allowDrag}
          on:touchstart|nonpassive={(e) => handleTouchDragStart(e, idx)}
          ><DragVertical size={16} /></span
        >

        <div class="rl-body">
          <div class="rl-top-line">
            <span class="rl-name">{rule.name || "Unnamed Rule"}</span>
            <span class="rl-action rl-action--{rule.action}"
              >{actionLabel(rule.action)}</span
            >
          </div>
          <div class="rl-meta">
            <span class="rl-users">{usersSummary(rule.users)}</span>
            {#if rule.domain_patterns && rule.domain_patterns.length > 0}
              <span class="rl-badge"
                >{rule.domain_patterns.length} pattern{rule.domain_patterns
                  .length > 1
                  ? "s"
                  : ""}</span
              >
            {/if}
            {#if rule.domain_lists && rule.domain_lists.length > 0}
              <span class="rl-badge rl-badge--list"
                >{rule.domain_lists.length} list{rule.domain_lists.length > 1
                  ? "s"
                  : ""}</span
              >
            {/if}
            {#if rule.mitm_action === "enable"}
              <span class="rl-badge rl-badge--mitm">MITM</span>
            {/if}
          </div>
        </div>

        <!-- svelte-ignore a11y-no-static-element-interactions a11y-click-events-have-key-events -->
        <span
          class="rl-toggle"
          on:click|stopPropagation
          on:keydown|stopPropagation
        >
          <Toggle
            size="sm"
            toggled={rule.enabled}
            hideLabel
            labelA=""
            labelB=""
            on:toggle={() => toggleEnabled(rule, idx)}
          />
        </span>

        <!-- svelte-ignore a11y-no-static-element-interactions -->
        <span
          class="rl-chevron-btn"
          role="button"
          tabindex="0"
          title="Edit rule"
          on:click|stopPropagation={() => openRule(rule)}
          on:keydown|stopPropagation={(e) => {
            if (e.key === "Enter" || e.key === " ") openRule(rule);
          }}
          on:touchstart|passive={chevronTouchStart}
          on:touchend|preventDefault={(e) => chevronTouchEnd(e, rule)}
          ><ChevronRight size={20} /></span
        >
      </div>
    {/each}
  </div>
{/if}

{#if saving}
  <div style="margin-top: 12px;">
    <InlineLoading description="Saving..." />
  </div>
{/if}

<style>
  .rl-topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  .rl-info {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.85rem;
    color: #6f6f6f;
  }

  .rl-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
    background: #e0e0e0;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    overflow: hidden;
  }

  .rl-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 14px;
    background: #fff;
    cursor: default;
    transition: background-color 0.12s;
    -webkit-tap-highlight-color: transparent;
  }
  .rl-disabled {
    opacity: 0.65;
  }
  .rl-dragging {
    opacity: 0.35;
  }
  .rl-drag-over {
    box-shadow: inset 0 2px 0 0 #0f62fe;
  }

  .rl-drag {
    cursor: grab;
    color: #a8a8a8;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    padding: 4px 2px;
    border-radius: 4px;
    touch-action: none;
  }
  .rl-drag:hover {
    color: #525252;
    background: #e0e0e0;
  }
  .rl-drag:active {
    cursor: grabbing;
    color: #525252;
  }

  .rl-toggle {
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }

  .rl-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .rl-top-line {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .rl-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: #161616;
    word-break: break-word;
  }

  .rl-action {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    padding: 1px 8px;
    border-radius: 10px;
    flex-shrink: 0;
  }
  .rl-action--allow {
    background: #d0e2ff;
    color: #0043ce;
  }
  .rl-action--block {
    background: #ffd7d9;
    color: #a2191f;
  }

  .rl-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    font-size: 0.75rem;
    color: #6f6f6f;
  }

  .rl-users {
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .rl-badge {
    background: #e0e0e0;
    color: #393939;
    padding: 1px 7px;
    border-radius: 8px;
    font-size: 0.6875rem;
    white-space: nowrap;
  }
  .rl-badge--list {
    background: #d0e2ff;
    color: #0043ce;
  }
  .rl-badge--mitm {
    background: #e8daff;
    color: #6929c4;
  }

  .rl-chevron-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 40px;
    height: 40px;
    margin: -8px -6px -8px 0;
    border-radius: 50%;
    color: #a8a8a8;
    cursor: pointer;
    transition:
      background-color 0.12s,
      color 0.12s;
    -webkit-tap-highlight-color: transparent;
  }
  .rl-chevron-btn:hover {
    background: #e0e0e0;
    color: #525252;
  }
  .rl-chevron-btn:active {
    background: #c6c6c6;
    color: #161616;
  }

  @media (max-width: 671px) {
    .rl-row {
      padding: 10px 10px;
      gap: 8px;
    }
    .rl-drag {
      color: #525252;
    }
    .rl-chevron-btn {
      width: 44px;
      height: 44px;
    }
  }
</style>
