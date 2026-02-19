<script lang="ts">
  import {
    SideNavDivider,
    SideNavItems,
    SideNavLink,
    SideNavMenu,
  } from "carbon-components-svelte";
  import { menuItems } from "../menu";
  import { store } from "../store/apistore";
  import { afterUpdate } from "svelte";
  import { gsNavigate } from "../lib/navigate";
  $: loggedIn = $store.api.loggedIn;

  let menuItemsToRender = [...menuItems];
  afterUpdate(() => {
    if (loggedIn) {
      menuItemsToRender = [...menuItems];
    } else {
      menuItemsToRender = [];
    }
  });

  function navigateTo(href) {
    gsNavigate(href);
    // App.svelte decides whether to close (mobile only)
    document.dispatchEvent(new CustomEvent("closesidenav"));
  }
</script>

<SideNavItems>
  {#each menuItemsToRender as item}
    {#if item.type === "link"}
      <SideNavLink
        icon={item.icon}
        text={item.text}
        isSelected={item.isSelected}
        on:click={() => navigateTo(item.href)}
      />
    {:else if item.type === "menu"}
      <SideNavMenu icon={item.icon} text={item.text}>
        {#each item.children as child}
          <SideNavLink
            icon={child.icon}
            text={child.text}
            on:click={() => navigateTo(child.href)}
          />
        {/each}
      </SideNavMenu>
    {/if}
  {/each}
  <SideNavDivider />
</SideNavItems>
