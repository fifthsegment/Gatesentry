<script lang="ts">
  import { Modal, ModalBody } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  export let open = false;
  export let on: { close: () => void } = { close: () => {} };
  export let title = "";
  export let children = null;
</script>

<Modal
  bind:open
  on:close={on.close}
  modalHeading={title}
  primaryButtonDisabled={true}
  >{#if children}
    <svelte:component this={children} />
  {/if}</Modal
>
