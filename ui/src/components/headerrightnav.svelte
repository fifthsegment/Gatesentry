<script lang="ts">
  import { store } from "../store/apistore";
  import { _ } from "svelte-i18n";

  export let userProfilePanelOpen;
  import {
    Button,
    Checkbox,
    ComposedModal,
    HeaderAction,
    HeaderGlobalAction,
    HeaderPanelDivider,
    HeaderPanelLink,
    HeaderPanelLinks,
    HeaderUtilities,
    ModalBody,
    ModalFooter,
    ModalHeader,
  } from "carbon-components-svelte";
  import { SettingsAdjust, UserAvatarFilledAlt } from "carbon-icons-svelte";
  import { afterUpdate } from "svelte";
  import ConnectedGeneralSettingInputs from "./connectedGeneralSettingInputs.svelte";
  import { navigate } from "svelte-routing";

  $: loggedIn = $store.api.loggedIn;
  let checked = false;

  let bindedUpdate;
  let modalOpen;

  let onLogout = () => {
    // navigate("/login");
    store.logout();
    userProfilePanelOpen = false;
    modalOpen = false;
    navigate("/login");
  };
</script>

<HeaderUtilities>
  {#if loggedIn}
    <HeaderAction
      bind:isOpen={userProfilePanelOpen}
      icon={UserAvatarFilledAlt}
      closeIcon={UserAvatarFilledAlt}
    >
      <HeaderPanelLinks>
        <HeaderPanelDivider>Logged in as admin</HeaderPanelDivider>
        <HeaderPanelLink
          on:click={() => {
            modalOpen = true;
          }}>{$_("Change password")}</HeaderPanelLink
        >
        <HeaderPanelLink on:click={onLogout}>Logout</HeaderPanelLink>
      </HeaderPanelLinks>
    </HeaderAction>

    <ComposedModal open={modalOpen}>
      <ModalHeader title={$_("Update password")} />

      <ModalBody hasForm>
        {#if loggedIn}
          <ConnectedGeneralSettingInputs
            keyName="admin_password"
            helperText={$_("Leave blank to keep the current password")}
            type="password"
            title={$_("Password")}
            labelText={$_("Password")}
            disableOnblur={true}
            bind:updateDataOnBackend={bindedUpdate}
          />
        {/if}
      </ModalBody>
      <ModalFooter
        secondaryButtonText="Proceed"
        primaryButtonDisabled={true}
        secondaryClass="button--primary"
        on:click:button--secondary={() => {
          modalOpen = false;
          bindedUpdate();
        }}
      />
    </ComposedModal>
  {/if}
</HeaderUtilities>
