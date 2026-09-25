<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import { Modal } from "carbon-components-svelte";

  export let open = false;
  export let title = "";
  export let label = "";
  export let primaryButtonText = "";
  export let secondaryButtonText = "";
  export let primaryButtonDisabled = false;
  export let hasForm = false;
  export let shouldSubmitOnEnter = false;
  export let danger = false;
  export let size: "xs" | "sm" | "lg" | undefined = undefined;
  export let preventCloseOnClickOutside = false;

  const dispatch = createEventDispatcher<{
    close: void;
    submit: void;
  }>();

  const close = () => {
    open = false;
    dispatch("close");
  };
</script>

{#if open}
  <Modal
    bind:open
    modalHeading={title}
    modalLabel={label}
    {primaryButtonText}
    {secondaryButtonText}
    {primaryButtonDisabled}
    {hasForm}
    {shouldSubmitOnEnter}
    {danger}
    {size}
    {preventCloseOnClickOutside}
    on:submit={() => dispatch("submit")}
    on:click:button--secondary={close}
    on:close={() => dispatch("close")}
  >
    <slot />
  </Modal>
{/if}
