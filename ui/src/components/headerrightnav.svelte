<script lang="ts">
  import { store } from "../store/apistore";

  export let userProfilePanelOpen;
  import {
    HeaderAction,
    HeaderGlobalAction,
    HeaderPanelDivider,
    HeaderPanelLink,
    HeaderPanelLinks,
    HeaderUtilities,
  } from "carbon-components-svelte";
  import { SettingsAdjust, UserAvatarFilledAlt } from "carbon-icons-svelte";
  import { afterUpdate } from "svelte";

  $: loggedIn = $store.api.loggedIn;
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
        <HeaderPanelLink on:click={store.logout}>Logout</HeaderPanelLink>
      </HeaderPanelLinks>
    </HeaderAction>
  {/if}
</HeaderUtilities>
