<script lang="ts">
  import { Modal, ModalBody } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  export let open = false;
  export let on: { close: () => void } = { close: () => {} };
  export let title = "";
  export let children = null;
  export let hasForm = false;
  export let shouldSubmitOnEnter = false;
</script>

{#if open}
  <div class="gatesentry-modal">
    <Modal
      {shouldSubmitOnEnter}
      {hasForm}
      bind:open
      on:close={on.close}
      modalHeading={title}
      primaryButtonDisabled={true}
      ><slot />
    </Modal>
  </div>
{/if}
