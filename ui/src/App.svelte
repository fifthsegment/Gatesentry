<script lang="ts">
  import { Content, SideNav } from "carbon-components-svelte";
  import { Router, Route } from "svelte-routing";
  import { init, register } from "svelte-i18n";
  import { onMount } from "svelte";
  import { store } from "./store/apistore";
  import { getBasePath, gsNavigate } from "./lib/navigate";
  import Globalheader from "./components/globalheader.svelte";
  import Sidenavmenu from "./components/sidenavmenu.svelte";
  import Notifications from "./components/notifications.svelte";
  import AuthShell from "./components/layout/AuthShell.svelte";
  import ResourceState from "./components/layout/ResourceState.svelte";
  import Login from "./routes/login/login.svelte";
  import Setup from "./routes/setup/setup.svelte";
  import Logs from "./routes/logs/logs.svelte";
  import Filter from "./routes/filter/filter.svelte";
  import Settings from "./routes/settings/settings.svelte";
  import Services from "./routes/services/services.svelte";
  import Home from "./routes/home/home.svelte";
  import Dns from "./routes/dns/dns.svelte";
  import Stats from "./routes/stats/stats.svelte";
  import AI from "./routes/ai/ai.svelte";
  import Users from "./routes/users/users.svelte";
  import Rules from "./routes/rules/rules.svelte";
  import Devices from "./routes/devices/devices.svelte";
  import NotFound from "./routes/notfound/notfound.svelte";

  export let url = "";

  type AppState = "checking" | "setup" | "login" | "authenticated" | "error";

  register("en", () => import("./language/en.json"));
  const localization = init({ initialLocale: "en", fallbackLocale: "en" });

  const navigationBreakpoint = 1056;

  let state: AppState = "checking";
  let isSideNavOpen = false;
  let userProfilePanelOpen = false;
  let pathname = "/";
  let windowWidth = 0;

  $: isCompactNavigation = windowWidth < navigationBreakpoint;
  $: sideNavVisible = !isCompactNavigation || isSideNavOpen;

  const routePath = () => {
    const base = getBasePath();
    const browserPath = typeof window === "undefined" ? url : window.location.pathname;
    if (base && (browserPath === base || browserPath.startsWith(base + "/"))) {
      return browserPath.slice(base.length) || "/";
    }
    return browserPath || "/";
  };

  const syncPathname = () => {
    pathname = routePath();
    isSideNavOpen = false;
  };

  const closeSideNavigation = () => {
    isSideNavOpen = false;
    if (isCompactNavigation && typeof window !== "undefined") {
      window.requestAnimationFrame(() => {
        document
          .querySelector<HTMLButtonElement>(".bx--header__menu-toggle")
          ?.focus();
      });
    }
  };

  async function checkSession() {
    state = "checking";
    try {
      const response = await fetch(getBasePath() + "/api/setup/status");
      if (!response.ok) throw new Error("setup status failed");
      const status = await response.json();
      if (!status.complete) {
        store.logout();
        state = "setup";
        if (routePath() !== "/setup") gsNavigate("/setup", { replace: true });
        return;
      }

      const valid = await $store.api.verifyToken();
      if (valid) {
        store.refresh();
        state = "authenticated";
        if (routePath() === "/login" || routePath() === "/setup") {
          gsNavigate("/", { replace: true });
        }
      } else {
        state = "login";
        if (routePath() !== "/login") gsNavigate("/login", { replace: true });
      }
    } catch {
      state = "error";
    }
  }

  onMount(() => {
    syncPathname();
    window.addEventListener("popstate", syncPathname);
    return () => window.removeEventListener("popstate", syncPathname);
  });

  Promise.resolve(localization).then(checkSession).catch(() => (state = "error"));
</script>

<svelte:window bind:innerWidth={windowWidth} />

<Router {url} basepath={getBasePath()}>
  {#if state === "checking"}
    <AuthShell title="GateSentry" description="Checking this installation…">
      <ResourceState state="loading" message="Checking setup and session status" />
    </AuthShell>
  {:else if state === "error"}
    <AuthShell title="GateSentry" description="The administration interface is unavailable.">
      <ResourceState
        state="error"
        title="Unable to read GateSentry setup status."
        message="Check that the GateSentry service is running, then try again."
        retryLabel="Retry status check"
        onRetry={checkSession}
      />
    </AuthShell>
  {:else if state === "setup"}
    <Route path="/setup" component={Setup} />
  {:else if state === "login"}
    <Route path="/login">
      <Login on:authenticated={() => (state = "authenticated")} />
    </Route>
  {:else}
    <div class="app-frame">
      <Globalheader
        bind:isSideNavOpen
        bind:userProfilePanelOpen
        {pathname}
        on:loggedout={() => (state = "login")}
      />
      <SideNav
        bind:isOpen={isSideNavOpen}
        rail
        expansionBreakpoint={navigationBreakpoint}
        ariaLabel="Primary navigation"
        aria-hidden={!sideNavVisible}
        inert={!sideNavVisible}
      >
        <Sidenavmenu {pathname} onNavigate={closeSideNavigation} />
      </SideNav>
      <Content id="main-content">
        <Route path="/dns" component={Dns} />
        <Route path="/logs" component={Logs} />
        <Route path="/settings" component={Settings} />
        <Route path="/blockedkeywords"><Filter type="blockedkeywords" /></Route>
        <Route path="/blockedfiletypes"><Filter type="blockedfiletypes" /></Route>
        <Route path="/excludeurls"><Filter type="excludeurls" /></Route>
        <Route path="/excludehosts"><Filter type="excludehosts" /></Route>
        <Route path="/services" component={Services} />
        <Route path="/rules" component={Rules} />
        <Route path="/devices" component={Devices} />
        <Route path="/stats" component={Stats} />
        <Route path="/ai" component={AI} />
        <Route path="/users" component={Users} />
        <Route path="/" component={Home} />
        <Route component={NotFound} />
      </Content>
      <Notifications />
    </div>
  {/if}
</Router>
