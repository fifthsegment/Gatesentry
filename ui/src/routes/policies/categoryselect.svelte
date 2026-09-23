<script lang="ts">
  import { Checkbox } from "carbon-components-svelte";
  import { createEventDispatcher } from "svelte";
  import type { CategoryStatus } from "./policymodel";

  // The same control selects the gateway-wide categories and one group's
  // categories, so the two cannot drift apart in behavior or in styling.
  export let categories: CategoryStatus[] = [];
  export let selection: string[] = [];
  export let disabled = false;
  // A rule picks categories inside an already busy form, so the compact
  // layout drops the descriptions and packs the boxes tighter.
  export let compact = false;

  const dispatch = createEventDispatcher<{ change: string[] }>();

  // The selection is an explicit array rather than a bound group, so the value
  // the page renders is always the value the API stored.
  function toggle(id: string, checked: boolean) {
    if (checked === selection.includes(id)) return;
    dispatch("change", checked ? [...selection, id] : selection.filter((candidate) => candidate !== id));
  }

  function coverageLabel(category: CategoryStatus): string {
    if (!category.domain_count) return "Not downloaded yet";
    return category.domain_count.toLocaleString() + " domains";
  }
</script>

<div class="category-grid" class:compact>
  {#each categories as category (category.id)}
    <div class="category-item">
      <div class="category-line">
        <Checkbox
          labelText={category.name}
          checked={selection.includes(category.id)}
          {disabled}
          on:check={(event) => toggle(category.id, event.detail)}
        />
        <span class="category-count">{coverageLabel(category)}</span>
      </div>
      {#if !compact}
        <p class="category-note">{category.description}</p>
      {/if}
    </div>
  {/each}
</div>

<style>
  /* The entries are short, so a laptop fits two or three columns instead of
     one long list. Carbon v10's compiled g10 theme exposes literal colors
     rather than --cds-* custom properties: #525252 is text-02. */
  .category-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
    column-gap: 2rem;
  }
  .category-grid.compact {
    grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
    margin-bottom: 0.5rem;
  }
  .category-line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
  }
  /* Carbon's form item carries a 1rem bottom margin, which is taller than the
     dense selection list needs. */
  .category-grid .category-item :global(.bx--form-item) {
    margin-bottom: 0;
  }
  /* The description says what a category covers and, more usefully, what it
     leaves alone, so it sits under the box instead of in a tooltip. */
  .category-note {
    margin: 0 0 0.75rem;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
  .category-count {
    flex-shrink: 0;
    color: #525252;
    font-size: 0.75rem;
    line-height: 1.125rem;
  }
</style>
