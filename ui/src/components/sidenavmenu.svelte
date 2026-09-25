<script lang="ts">
  import {
    SideNavDivider,
    SideNavItems,
    SideNavLink,
    SideNavMenu,
    SideNavMenuItem,
  } from "carbon-components-svelte";
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
  export let onNavigate: () => void = () => {};
  const location = useLocation();
  $: activePath = $location?.pathname ? currentRoute($location.pathname) : pathname;

  const navigate = (event: MouseEvent, href: string) => {
    event.preventDefault();
    gsNavigate(href);
    onNavigate();
  };

  const linkHref = (item: (typeof menuItems)[number]) =>
    item.type === "link" ? item.href : "/";
</script>

<SideNavItems>
  {#each menuItems as item}
    {#if item.type === "link"}
      <SideNavLink
        icon={item.icon}
        text={item.text}
        href={routeHref(item.href)}
        isSelected={isRouteActive(item.href, activePath)}
        on:click={(event) => navigate(event, linkHref(item))}
      />
    {:else}
      <SideNavMenu
        icon={item.icon}
        text={item.text}
        expanded={isMenuActive(item, activePath)}
      >
        {#each item.children as child}
          <SideNavMenuItem
            text={child.text}
            href={routeHref(child.href)}
            isSelected={isRouteActive(child.href, activePath)}
            on:click={(event) => navigate(event, child.href)}
          />
        {/each}
      </SideNavMenu>
    {/if}
  {/each}
  <SideNavDivider />
</SideNavItems>
