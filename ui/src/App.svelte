<script lang="ts">
  import {
    Header,
    SkipToContent,
    Content,
    SideNav,
  } from "carbon-components-svelte";
  import { afterUpdate } from "svelte";

  import { Router, Route } from "svelte-routing";
  import Login from "./routes/login/login.svelte";
  import Logs from "./routes/logs/logs.svelte";
  import Headermenu from "./components/headermenu.svelte";
  import Sidenavmenu from "./components/sidenavmenu.svelte";
  import Headerrightnav from "./components/headerrightnav.svelte";
  import { store } from "./store/apistore";
  import { navigate } from "svelte-routing/src/history";
  import Filter from "./routes/filter/filter.svelte";
  import Notifications from "./components/notifications.svelte";
  import { setupI18n } from "./language/i18n";
  import Settings from "./routes/settings/settings.svelte";
  import Switches from "./routes/switches/switches.svelte";
  import Home from "./routes/home/home.svelte";
  import Dns from "./routes/dns/dns.svelte";
  import Stats from "./routes/stats/stats.svelte";

  import { register, init, _ } from "svelte-i18n";
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
  let version = "1.8.0";
  let userProfilePanelOpen = false;
  // setupI18n();

  $: loggedIn = $store.api.loggedIn;

  $: {
    if (loaded) {
      $store.api.verifyToken().then((isValid) => {
        if (isValid) {
          store.refresh();
        }
      });
      if (!loggedIn) {
        navigate("/login");
      }
    }
  }

  // afterUpdate(() => {
  //   if (!loggedIn) {
  //     navigate("/login");
  //   }
  // });
</script>

<Router {url}>
  {#await setupResult}
    Loading...
  {:then}
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

    <SideNav bind:isOpen={isSideNavOpen} rail>
      <Sidenavmenu />
    </SideNav>

    <Content>
      <div>
        <Route path="/login" component={Login} />
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
        <Route path="/blockedurls">
          <Filter type="blockedurls" />
        </Route>
        <Route path="/excludehosts">
          <Filter type="excludehosts" />
        </Route>
        <Route path="/switches">
          <Switches />
        </Route>
        <Route path="/stats">
          <Stats />
        </Route>
        <Route path="/" component={Home} />
      </div>

      <Notifications />
    </Content>
  {:catch error}
    <!-- <p style="color: red">{error.message}</p> -->
    Error: Unable to load localization.
  {/await}
</Router>
