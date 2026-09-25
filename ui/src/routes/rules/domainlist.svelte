<script lang="ts">
  import { Button, Tag, TextInput } from "carbon-components-svelte";
  import { createEventDispatcher } from "svelte";

  // One editable list of domain patterns. The blocked list, the allowed list,
  // and a rule's own domains all use it, so they accept the same input.
  export let entries: string[] = [];
  export let label = "Domain";
  export let tagType: "red" | "green" | "outline" = "outline";

  const dispatch = createEventDispatcher<{ change: string[] }>();
  let draft = "";

  // An administrator pastes whatever the address bar shows, so the scheme,
  // path, and port are dropped here; the API applies the same rule and
  // rejects anything that still is not a domain.
  function clean(raw: string): string {
    let value = raw.trim().toLowerCase();
    const scheme = value.indexOf("://");
    if (scheme >= 0) value = value.slice(scheme + 3);
    value = value.split(/[/?#]/)[0].replace(/:\d+$/, "").replace(/\.$/, "");
    return value;
  }

  function add() {
    const values = draft
      .split(/[\s,]+/)
      .map(clean)
      .filter(Boolean);
    const next = [...entries];
    for (const value of values) {
      if (!next.includes(value)) next.push(value);
    }
    draft = "";
    if (next.length !== entries.length) dispatch("change", next);
  }

  function remove(entry: string) {
    dispatch(
      "change",
      entries.filter((candidate) => candidate !== entry),
    );
  }
</script>

<div class="list-entry">
  <TextInput
    labelText={label}
    placeholder="tiktok.com (covers its subdomains)"
    bind:value={draft}
    on:keydown={(event) => event.key === "Enter" && add()}
  />
  <Button size="small" kind="tertiary" on:click={add}>Add</Button>
</div>
{#if entries.length}
  <div class="tags">
    {#each entries as entry (entry)}
      <Tag filter size="sm" type={tagType} on:close={() => remove(entry)}
        >{entry}</Tag
      >
    {/each}
  </div>
{/if}

<style>
  .list-entry {
    display: flex;
    align-items: flex-end;
    gap: 1rem;
  }
  .list-entry :global(.bx--text-input-wrapper) {
    flex: 1;
  }
  .list-entry :global(.bx--form-item) {
    margin-bottom: 0;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin: 0.5rem 0;
  }
  .tags :global(.bx--tag) {
    margin: 0;
  }
  @media (max-width: 30rem) {
    .list-entry {
      align-items: stretch;
      flex-direction: column;
      gap: 0.5rem;
    }
    .list-entry :global(.bx--btn) {
      width: 100%;
      max-width: none;
    }
  }
</style>
