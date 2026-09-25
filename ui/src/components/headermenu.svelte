<script lang="ts">
  import { HeaderNav, HeaderNavItem, HeaderNavMenu } from "carbon-components-svelte";
  import { useLocation } from "svelte-routing";
  import {
    currentRoute,
    isMenuActive,
    isRouteActive,
    menuItems,
    routeHref,
  } from "../menu";
  import { gsNavigate } from "../lib/navigate";

  export let pathname = "/";
  const location = useLocation();
  $: activePath = $location?.pathname ? currentRoute($location.pathname) : pathname;

  const navigate = (event: MouseEvent, href: string) => {
    event.preventDefault();
    gsNavigate(href);
  };

  const linkHref = (item: (typeof menuItems)[number]) =>
    item.type === "link" ? item.href : "/";
</script>

<HeaderNav aria-label="Primary navigation">
  {#each menuItems as item}
    {#if item.type === "link"}
      <HeaderNavItem
        text={item.text}
        href={routeHref(item.href)}
        isSelected={isRouteActive(item.href, activePath)}
        on:click={(event) => navigate(event, linkHref(item))}
      />
    {:else}
      <HeaderNavMenu
        text={item.text}
        href={routeHref(item.children[0].href)}
        expanded={isMenuActive(item, activePath)}
      >
        {#each item.children as child}
          <HeaderNavItem
            text={child.text}
            href={routeHref(child.href)}
            isSelected={isRouteActive(child.href, activePath)}
            on:click={(event) => navigate(event, child.href)}
          />
        {/each}
      </HeaderNavMenu>
    {/if}
  {/each}
</HeaderNav>
