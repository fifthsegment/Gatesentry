<script lang="ts">
  import { Modal } from "carbon-components-svelte";

  export let open = false;
  export let title = "Confirm action";
  export let label = "Confirmation";
  export let confirmText = "Confirm";
  export let cancelText = "Cancel";
  export let danger = false;
  export let busy = false;

  const close = () => {
    if (!busy) open = false;
  };
</script>

<Modal
  bind:open
  {danger}
  alert
  modalHeading={title}
  modalLabel={label}
  primaryButtonText={busy ? "Working…" : confirmText}
  primaryButtonDisabled={busy}
  secondaryButtonText={cancelText}
  shouldSubmitOnEnter={false}
  preventCloseOnClickOutside={busy}
  on:click:button--primary
  on:submit
  on:click:button--secondary={close}
  on:close
>
  <slot />
</Modal>
