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

  let isSideNavOpen = false;
  let version = "1.8.0";
  let userProfilePanelOpen = false;
  let url = "/auth/verify";

  setupI18n();

  $: loggedIn = $store.api.loggedIn;

  $store.api.verifyToken().then((isValid) => {
    if (isValid) {
      store.refresh();
    }
  });

  afterUpdate(() => {
    if (!loggedIn) {
      navigate("/login");
    }
  });
</script>

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
  <Router {url}>
    <div>
      <Route path="/login" component={Login} />
      <Route path="/" component={Home}></Route>
      <Route path="/dns" component={Dns}></Route>
      <Route path="/logs" component={Logs} />
      <Route path="/settings" component={Settings} />
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
    </div>
  </Router>
  <Notifications />
</Content>
