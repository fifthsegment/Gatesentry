<script lang="ts">
  import {
    Header,
    SkipToContent,
    Content,
    SideNav,
  } from "carbon-components-svelte";
  import { onMount, onDestroy } from "svelte";

  import { Router, Route } from "svelte-routing";
  import Login from "./routes/login/login.svelte";
  import Logs from "./routes/logs/logs.svelte";
  import Sidenavmenu from "./components/sidenavmenu.svelte";
  import Headerrightnav from "./components/headerrightnav.svelte";
  import { store } from "./store/apistore";
  import { gsNavigate, getBasePath } from "./lib/navigate";
  import Filter from "./routes/filter/filter.svelte";
  import Notifications from "./components/notifications.svelte";
  import Settings from "./routes/settings/settings.svelte";
  import Home from "./routes/home/home.svelte";
  import Dns from "./routes/dns/dns.svelte";
  import Stats from "./routes/stats/stats.svelte";
  import AI from "./routes/ai/ai.svelte";

  import { register, init, _ } from "svelte-i18n";
  import Users from "./routes/users/users.svelte";
  import Globalheader from "./components/globalheader.svelte";
  import Rules from "./routes/rules/rules.svelte";
  import Devices from "./routes/devices/devices.svelte";
  import DomainLists from "./routes/domainlists/domainlists.svelte";
  export let url = "";

  let loaded = false;
  async function setup() {
    register("en", () => import("./language/en.json"));

    await Promise.allSettled([
      init({ initialLocale: "en", fallbackLocale: "en" }),
    ]);
    loaded = true;
    return true;
  }

  const setupResult = setup();

  // ── Sidebar state ──
  const DESKTOP_BREAKPOINT = 1056;
  const STORAGE_KEY = "gs_sideNavExpanded";

  let innerWidth = 0;
  $: isDesktop = innerWidth >= DESKTOP_BREAKPOINT;

  // Read persisted state — default to expanded on desktop
  let isSideNavOpen = (() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored !== null) return stored === "true";
    } catch (_) {}
    return true;
  })();

  // Persist whenever the user toggles (only on desktop — mobile is transient)
  $: if (typeof window !== "undefined" && isDesktop) {
    try {
      localStorage.setItem(STORAGE_KEY, String(isSideNavOpen));
    } catch (_) {}
  }

  let version = "-";
  let userProfilePanelOpen = false;
  let tokenVerified = false;

  $: loggedIn = $store.api.loggedIn;

  $: {
    if (loaded) {
      if (!tokenVerified) {
        $store.api.verifyToken().then((isValid) => {
          tokenVerified = true;
          if (isValid) {
            store.refresh();
          }
        });
      }

      if (!loggedIn) {
        gsNavigate("/login");
      }
    }
  }

  // Mobile: close sidebar when a menu item is clicked
  function handleCloseSideNav() {
    if (!isDesktop) {
      isSideNavOpen = false;
    }
  }

  onMount(() => {
    document.addEventListener("closesidenav", handleCloseSideNav);
  });

  onDestroy(() => {
    document.removeEventListener("closesidenav", handleCloseSideNav);
  });
</script>

<svelte:window bind:innerWidth />

<Router {url} basepath={getBasePath()}>
  {#await setupResult}
    Loading...
  {:then}
    {#if !loggedIn}
      <!-- Login is outside the dashboard shell -->
      <Route path="/login" component={Login} />
      <Route path="*" component={Login} />
      <Notifications />
    {:else}
      <div
        class="gs-app"
        class:gs-nav-open={isSideNavOpen}
        class:gs-desktop={isDesktop}
      >
        <Globalheader bind:isSideNavOpen bind:userProfilePanelOpen />

        <SideNav bind:isOpen={isSideNavOpen} fixed={isDesktop}>
          <Sidenavmenu />
        </SideNav>

        <Content>
          <div>
            <Route path="/dns" component={Dns}></Route>
            <Route path="/domainlists" component={DomainLists}></Route>
            <Route path="/logs" component={Logs} />
            <Route path="/settings">
              <Settings />
            </Route>
            <Route path="/blockedkeywords">
              <Filter />
            </Route>
            <Route path="/rules">
              <Rules />
            </Route>
            <Route path="/devices">
              <Devices />
            </Route>
            <Route path="/stats">
              <Stats />
            </Route>
            <Route path="/ai">
              <AI />
            </Route>
            <Route path="/users">
              <Users />
            </Route>
            <Route path="/" component={Home} />
          </div>

          <Notifications />
        </Content>
      </div>
    {/if}
  {:catch error}
    Error: Unable to load localization.
  {/await}
</Router>

<style>
  /* Desktop sidebar width */
  :global(.gs-desktop .bx--side-nav) {
    width: 200px;
  }
  /* Desktop: fixed sidebar pushes content. Collapse hides it and content fills. */
  :global(.gs-desktop.gs-nav-open .bx--side-nav) {
    transform: translateX(0);
  }
  :global(.gs-desktop:not(.gs-nav-open) .bx--side-nav) {
    transform: translateX(-200px);
  }
  :global(.gs-desktop.gs-nav-open .bx--content) {
    margin-left: 200px !important;
    transition: margin-left 0.11s cubic-bezier(0.2, 0, 1, 0.9);
  }
  :global(.gs-desktop:not(.gs-nav-open) .bx--content) {
    margin-left: 0 !important;
    transition: margin-left 0.11s cubic-bezier(0.2, 0, 1, 0.9);
  }
  /* Desktop: no overlay backdrop */
  :global(.gs-desktop .bx--side-nav__overlay) {
    display: none;
  }
  /* Mobile: overlay mode — Content NEVER shifts, sidebar floats above */
  :global(.gs-app:not(.gs-desktop) .bx--content) {
    margin-left: 0 !important;
    transition: none;
  }
</style>
