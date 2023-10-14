<script lang="ts">
  import { Header, SkipToContent } from "carbon-components-svelte";
  import Headermenu from "./headermenu.svelte";
  import Headerrightnav from "./headerrightnav.svelte";
  import { onMount } from "svelte";
  import { store } from "../store/apistore";
  export let isSideNavOpen = false;
  export let userProfilePanelOpen = false;
  let version = "";

  onMount(async () => {
    const data = await $store.api.doCall("/about");
    version = data.version;
  });
</script>

<Header
  company="Gatesentry"
  platformName={version}
  bind:isSideNavOpen
  persistentHamburgerMenu={true}
>
  <svelte:fragment slot="skip-to-content">
    <SkipToContent />
  </svelte:fragment>
  <Headermenu />

  <Headerrightnav {userProfilePanelOpen} />
</Header>
