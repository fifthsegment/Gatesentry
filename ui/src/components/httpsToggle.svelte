<script lang="ts">
  import Toggle from "./toggle.svelte";
  import { _ } from "svelte-i18n";
  import {
    ComposedModal,
    ModalBody,
    ModalFooter,
    ModalHeader,
  } from "carbon-components-svelte";
  import Privacynotice from "./privacynotice.svelte";

  let modalOpen = false;

  let enable_https_filtering = "";
</script>

<div
  role="button"
  tabindex="0"
  on:click={() => {
    if (enable_https_filtering == "false") {
      modalOpen = true;
    }
  }}
  on:keydown={(e) => {
    if (
      (e.key === "Enter" || e.key === " ") &&
      enable_https_filtering == "false"
    ) {
      modalOpen = true;
    }
  }}
>
  <Toggle
    bind:settingValue={enable_https_filtering}
    settingName="enable_https_filtering"
    label={$_("HTTPS Filtering")}
    labelA={$_("Disabled")}
    labelB={$_("Enabled")}
    noNotification={true}
  />
</div>

<ComposedModal open={modalOpen}>
  <ModalHeader label="Privacy policy" title="" />
  <ModalBody hasForm>
    <Privacynotice />
  </ModalBody>
  <ModalFooter
    secondaryButtonText={$_("I have reviewed the policy")}
    secondaryClass="bx--btn--primary"
    on:click:button--secondary={() => {
      modalOpen = false;
    }}
  />
</ComposedModal>
