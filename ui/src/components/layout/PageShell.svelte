<script lang="ts">
  export let title = "";
  export let description = "";
  export let narrow = false;
</script>

<div class:page-shell--narrow={narrow} class="page-shell">
  <header class="page-shell__header">
    <div class="page-shell__heading">
      <slot name="breadcrumb" />
      <h1>{title}</h1>
      {#if description}
        <p>{description}</p>
      {/if}
    </div>
    {#if $$slots.actions}
      <div class="page-shell__actions"><slot name="actions" /></div>
    {/if}
  </header>
  <div class="page-shell__content"><slot /></div>
</div>
