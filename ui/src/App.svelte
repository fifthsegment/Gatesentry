<script lang="ts">
  import {
    Header,
    SkipToContent,
    Content,
    SideNav,
  } from "carbon-components-svelte";
  import { afterUpdate, onMount } from "svelte";

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
  let version = "-";
  let userProfilePanelOpen = false;
  let tokenVerified = false;
  // setupI18n();

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
        navigate("/login");
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

<Router {url}>
  {#await setupResult}
    Loading...
  {:then}
    <Globalheader bind:isSideNavOpen bind:userProfilePanelOpen />

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
        <Route path="/services">
          <Services />
        </Route>
        <Route path="/rules">
          <Rules />
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
  {:catch error}
    <!-- <p style="color: red">{error.message}</p> -->
    Error: Unable to load localization.
  {/await}
</Router>
