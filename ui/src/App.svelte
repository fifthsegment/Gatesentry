<script lang="ts">
  import {
    Header,
    SkipToContent,
    Content,
    SideNav,
  } from "carbon-components-svelte";
  import { Router, Route } from "svelte-routing";
  import Login from "./routes/login/login.svelte";
  import Setup from "./routes/setup/setup.svelte";
  import Logs from "./routes/logs/logs.svelte";
  import Headermenu from "./components/headermenu.svelte";
  import Sidenavmenu from "./components/sidenavmenu.svelte";
  import Headerrightnav from "./components/headerrightnav.svelte";
  import { store } from "./store/apistore";
  import { gsNavigate, getBasePath } from "./lib/navigate";
  import Filter from "./routes/filter/filter.svelte";
  import Notifications from "./components/notifications.svelte";
  import Settings from "./routes/settings/settings.svelte";
  import Services from "./routes/services/services.svelte";
  import Home from "./routes/home/home.svelte";
  import Dns from "./routes/dns/dns.svelte";
  import Stats from "./routes/stats/stats.svelte";
  import AI from "./routes/ai/ai.svelte";

  import { register, init, _ } from "svelte-i18n";
  import Users from "./routes/users/users.svelte";
  import Globalheader from "./components/globalheader.svelte";
  import Rules from "./routes/rules/rules.svelte";
  import Devices from "./routes/devices/devices.svelte";
  export let url = "";

  let loaded = false;
  async function setup() {
    register("en", () => import("./language/en.json"));

    await Promise.allSettled([
      // TODO: add some more stuff you want to init ...
      init({ initialLocale: "en", fallbackLocale: "en" }),
    ]);
    loaded = true;
    return true;
  }

  const setupResult = setup();

  let isSideNavOpen = false;
  let userProfilePanelOpen = false;
  let tokenVerified = false;
  let tokenChecking = false;
  let setupChecked = false;
  let setupChecking = false;
  let setupStatusFailed = false;
  let setupRequired = false;
  // setupI18n();

  $: loggedIn = $store.api.loggedIn;

  function checkSetupStatus() {
    if (setupChecking) return;
    setupStatusFailed = false;
    setupChecking = true;
    fetch(getBasePath() + "/api/setup/status")
      .then((response) => {
        if (!response.ok) throw new Error("setup status failed");
        return response.json();
      })
      .then((status) => {
        setupRequired = !status.complete;
        setupChecked = true;
        if (setupRequired) {
          store.logout();
          gsNavigate("/setup");
        }
      })
      .catch(() => {
        // Leave routing in a stable fail-closed state. Only the explicit retry
        // below starts another request.
        setupStatusFailed = true;
      })
      .finally(() => setupChecking = false);
  }

  $: {
    if (loaded) {
      if (!setupChecked && !setupChecking && !setupStatusFailed) {
        checkSetupStatus();
      }
      if (setupChecked && !setupRequired && !tokenVerified && !tokenChecking) {
        tokenChecking = true;
        $store.api.verifyToken().then((isValid) => {
          tokenVerified = true;
          if (isValid) {
            store.refresh();
          } else if (setupChecked && !setupRequired) {
            gsNavigate("/login");
          }
        }).finally(() => tokenChecking = false);
      }

      if (setupChecked && tokenVerified && !setupRequired && !loggedIn) {
        gsNavigate("/login");
      }
    }
  }

  // afterUpdate(() => {
  //   if (!loggedIn) {
  //     navigate("/login");
  //   }
  // });
  // onMount(() => {});

  // $: {
  //   if (setupResult) {

  //   }
  // }
</script>

<Router {url} basepath={getBasePath()}>
  {#await setupResult}
    Loading...
  {:then}
    {#if setupStatusFailed}
      <main class="setup-status-error">
        <p role="alert">Unable to read GateSentry setup status.</p>
        <button type="button" on:click={checkSetupStatus}>Retry status check</button>
      </main>
    {:else}
    <Globalheader bind:isSideNavOpen bind:userProfilePanelOpen />

    <SideNav bind:isOpen={isSideNavOpen} rail>
      <Sidenavmenu />
    </SideNav>

    <Content>
      <div>
        <Route path="/login" component={Login} />
        <Route path="/setup" component={Setup} />
        <Route path="/dns" component={Dns}></Route>
        <Route path="/logs" component={Logs} />
        <Route path="/settings">
          <Settings />
        </Route>
        <Route path="/blockedkeywords">
          <Filter type="blockedkeywords" />
        </Route>
        <Route path="/blockedfiletypes">
          <Filter type="blockedfiletypes" />
        </Route>
        <Route path="/excludeurls">
          <Filter type="excludeurls" />
        </Route>
        <Route path="/excludehosts">
          <Filter type="excludehosts" />
        </Route>
        <Route path="/services">
          <Services />
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
    {/if}
  {:catch error}
    <!-- <p style="color: red">{error.message}</p> -->
    Error: Unable to load localization.
  {/await}
</Router>

<style>
  .setup-status-error { max-width: 32rem; margin: 15vh auto; padding: 1rem; }
  .setup-status-error p { margin-bottom: 1rem; }
</style>
