<script lang="ts">
  export let title = "";
  export let description = "";
</script>

<main class="auth-shell">
  <section class="auth-shell__card" aria-labelledby="auth-shell-title">
    <div class="auth-shell__brand">GateSentry</div>
    <header class="auth-shell__header">
      <h1 id="auth-shell-title">{title}</h1>
      {#if description}<p>{description}</p>{/if}
    </header>
    <slot />
  </section>
  {#if $$slots.footer}
    <footer class="auth-shell__footer"><slot name="footer" /></footer>
  {/if}
</main>
