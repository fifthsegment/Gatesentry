<script lang="ts">
  import {
    ComposedModal,
    ModalBody,
    ModalFooter,
    ModalHeader,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";

  import Privacynotice from "./privacynotice.svelte";
  import Toggle from "./toggle.svelte";

  let modalOpen = false;
  let enable_https_filtering = "";

  function reviewPrivacyBeforeEnable() {
    if (enable_https_filtering === "false") {
      modalOpen = true;
    }
  }
</script>

<div class="https-setting">
  <Toggle
    bind:settingValue={enable_https_filtering}
    settingName="enable_https_filtering"
    label={$_(
      "HTTPS inspection (decrypts traffic; needs the GateSentry certificate on each device)",
    )}
    labelA={$_("Disabled")}
    labelB={$_("Enabled")}
    noNotification={true}
    preClickEvent={reviewPrivacyBeforeEnable}
  />

  <p class="helper">
    {$_(
      "Only enable inspection after reviewing the privacy notice and installing the GateSentry certificate on protected devices.",
    )}
  </p>

  <ComposedModal open={modalOpen} on:close={() => (modalOpen = false)}>
    <ModalHeader label={$_("HTTPS inspection")} title={$_("Privacy notice")} />
    <ModalBody hasForm>
      <Privacynotice />
    </ModalBody>
    <ModalFooter
      secondaryButtonText={$_("Close")}
      secondaryClass="bx--btn--primary"
      on:click:button--secondary={() => {
        modalOpen = false;
      }}
    />
  </ComposedModal>
</div>

<style>
  .https-setting {
    min-width: 0;
  }

  .helper {
    max-width: 42rem;
    margin: 0.75rem 0 0;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
    line-height: 1.35;
  }
</style>
