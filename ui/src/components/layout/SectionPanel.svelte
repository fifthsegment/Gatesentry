<script lang="ts">
  export let title = "";
  export let description = "";
  export let ariaLabel = "";
</script>

<section class="section-panel" aria-label={ariaLabel || title || undefined}>
  {#if title || description || $$slots.actions}
    <header class="section-panel__header">
      <div>
        {#if title}<h2>{title}</h2>{/if}
        {#if description}<p>{description}</p>{/if}
      </div>
      {#if $$slots.actions}
        <div class="section-panel__actions"><slot name="actions" /></div>
      {/if}
    </header>
  {/if}
  <div class="section-panel__body"><slot /></div>
</section>
