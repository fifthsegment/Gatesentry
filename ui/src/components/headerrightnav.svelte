<script lang="ts">
  import {
    ComposedModal,
    HeaderAction,
    HeaderPanelDivider,
    HeaderPanelLink,
    HeaderPanelLinks,
    HeaderUtilities,
    ModalBody,
    ModalFooter,
    ModalHeader,
  } from "carbon-components-svelte";
  import { UserAvatarFilledAlt } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { createEventDispatcher } from "svelte";
  import { store } from "../store/apistore";
  import { gsNavigate } from "../lib/navigate";
  import ConnectedGeneralSettingInputs from "./connectedGeneralSettingInputs.svelte";

  export let userProfilePanelOpen = false;

  const dispatch = createEventDispatcher<{ loggedout: void }>();

  let updatePassword: (() => Promise<void>) | undefined;
  let modalOpen = false;

  const logout = () => {
    store.logout();
    userProfilePanelOpen = false;
    modalOpen = false;
    gsNavigate("/login", { replace: true });
    dispatch("loggedout");
  };

  const savePassword = () => {
    modalOpen = false;
    updatePassword?.();
  };
</script>

<HeaderUtilities>
  <HeaderAction
    bind:isOpen={userProfilePanelOpen}
    icon={UserAvatarFilledAlt}
    closeIcon={UserAvatarFilledAlt}
    text={$_("Account")}
  >
    <HeaderPanelLinks>
      <HeaderPanelDivider>
        {$store.api.username ? `Logged in as ${$store.api.username}` : "Logged in"}
      </HeaderPanelDivider>
      <HeaderPanelLink on:click={() => (modalOpen = true)}>
        {$_("Change password")}
      </HeaderPanelLink>
      <HeaderPanelLink on:click={logout}>{$_("Log out")}</HeaderPanelLink>
    </HeaderPanelLinks>
  </HeaderAction>

  <ComposedModal bind:open={modalOpen} on:submit={savePassword}>
    <ModalHeader title={$_("Update password")} />
    <ModalBody hasForm>
      <ConnectedGeneralSettingInputs
        keyName="admin_password"
        helperText={$_("Leave blank to keep the current password")}
        type="password"
        title={$_("Password")}
        labelText={$_("Password")}
        disableOnblur
        bind:updateDataOnBackend={updatePassword}
      />
    </ModalBody>
    <ModalFooter
      secondaryButtonText={$_("Cancel")}
      primaryButtonText={$_("Update password")}
      on:click:button--secondary={() => (modalOpen = false)}
    />
  </ComposedModal>
</HeaderUtilities>
