<script lang="ts">
  import { Button, InlineLoading, InlineNotification } from "carbon-components-svelte";

  export let state: "loading" | "empty" | "error" = "loading";
  export let title = "";
  export let message = "";
  export let retryLabel = "Retry";
  export let onRetry: (() => void) | null = null;
</script>

<div class="resource-state" class:resource-state--error={state === "error"}>
  {#if state === "loading"}
    <InlineLoading description={message || title || "Loading"} />
  {:else if state === "error"}
    <InlineNotification
      kind="error"
      hideCloseButton
      title={title || "Unable to load"}
      subtitle={message}
    />
    {#if onRetry}
      <Button size="small" kind="tertiary" on:click={onRetry}>{retryLabel}</Button>
    {/if}
  {:else}
    <h2>{title || "Nothing here yet"}</h2>
    {#if message}<p>{message}</p>{/if}
    <slot />
  {/if}
</div>
