<script lang="ts">
  import {
    HeaderNav,
    HeaderNavItem,
    HeaderNavMenu,
  } from "carbon-components-svelte";
  import { menuItems } from "../menu";
  import { store } from "../store/apistore";
  import { afterUpdate } from "svelte";
  import { navigate } from "svelte-routing";
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

<HeaderNav>
  {#each menuItemsToRender as item}
    {#if item.type === "link"}
      <HeaderNavItem
        text={item.text}
        on:click={() => {
          navigate(item.href);
        }}
      />
    {:else if item.type === "menu"}
      <HeaderNavMenu text={item.text}>
        {#each item.children as child}
          <HeaderNavItem
            text={child.text}
            on:click={() => {
              navigate(child.href);
            }}
          />
        {/each}
      </HeaderNavMenu>
    {/if}
  {/each}
</HeaderNav>
