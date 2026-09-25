<script lang="ts">
  import { Header, SkipToContent } from "carbon-components-svelte";
  import { onMount, createEventDispatcher } from "svelte";
  import { store } from "../store/apistore";
  import { routeHref } from "../menu";
  import Headermenu from "./headermenu.svelte";
  import Headerrightnav from "./headerrightnav.svelte";

  export let isSideNavOpen = false;
  export let userProfilePanelOpen = false;
  export let pathname = "/";
  let version = "";

  const dispatch = createEventDispatcher<{ loggedout: void }>();

  onMount(async () => {
    try {
      const data = await $store.api.doCall("/about");
      version = data?.version || "";
    } catch {
      version = "";
    }
  });
</script>

<Header
  company="GateSentry"
  platformName={version}
  href={routeHref("/")}
  bind:isSideNavOpen
  persistentHamburgerMenu
>
  <svelte:fragment slot="skip-to-content">
    <SkipToContent />
  </svelte:fragment>
  <Headermenu {pathname} />
  <Headerrightnav
    bind:userProfilePanelOpen
    on:loggedout={() => dispatch("loggedout")}
  />
</Header>
