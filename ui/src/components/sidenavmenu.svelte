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
  import { navigate } from "svelte-routing/src/history";
  $: loggedIn = $store.api.loggedIn;

  let menuItemsToRender = [...menuItems];
  afterUpdate(() => {
    if (loggedIn) {
      menuItemsToRender = [...menuItems];
    } else {
      menuItemsToRender = [];
    }
  });
</script>

<SideNavItems>
  {#each menuItemsToRender as item}
    {#if item.type === "link"}
      <SideNavLink
        icon={item.icon}
        text={item.text}
        isSelected={item.isSelected}
        on:click={() => {
          navigate(item.href);
        }}
      />
    {:else if item.type === "menu"}
      <SideNavMenu icon={item.icon} text={item.text}>
        {#each item.children as child}
          <SideNavLink
            icon={child.icon}
            text={child.text}
            on:click={() => {
              navigate(child.href);
            }}
          />
        {/each}
      </SideNavMenu>
    {/if}
  {/each}
  <SideNavDivider />
</SideNavItems>
