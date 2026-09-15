<script lang="ts">
  import {
    Button,
    ButtonSet,
    Column,
    FluidForm,
    Grid,
    PasswordInput,
    Row,
    TextInput,
  } from "carbon-components-svelte";
  import { ChevronRight, Close } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { getBasePath, gsNavigate } from "../../lib/navigate";
  import { afterUpdate, onMount } from "svelte";
  import { notificationstore } from "../../store/notifications";
  import { createNotificationError } from "../../lib/utils";
  import { _ } from "svelte-i18n";
  import DownloadCertificateLink from "../../components/downloadCertificateLink.svelte";

  let username: string = "";
  let password: string = "";
  let isEnabled: boolean = true;
  let loggedIn: boolean = false;

  let invalidMessage: string = "";
  let invalid: boolean = false;
  onMount(async () => {
    try {
      const response = await fetch(getBasePath() + "/api/setup/status");
      if (!response.ok) return;
      const status = await response.json();
      if (!status.complete) gsNavigate("/setup");
    } catch (ignoredError) {
      // App.svelte owns the full-page status error flow.
    }
  });
  let handleLogin = (e) => {
    e.preventDefault();
    var datatosend = { username: username, pass: password };
    $store.api.doCall("/auth/token", "post", datatosend).then(function (data) {
      if (data == undefined || data == null) {
        notificationstore.add(
          createNotificationError(
            { subtitle: $_("Unable to get a correct response from the api") },
            $_,
          ),
        );
        return;
      } else if (data?.Validated === true || data?.Validated === "true") {
        localStorage.removeItem("jwt");
        localStorage.setItem("jwt", data.Jwtoken);
        store.loginSuccesful(data.Jwtoken);
      } else {
        invalidMessage = $_("Invalid username or password");
        invalid = true;
      }
    });
  };

  const onCancel = () => {
    username = "";
    password = "";
  };

  $: {
    isEnabled = username.length > 0 || password.length > 0;

    loggedIn = $store.api.loggedIn;
  }

  afterUpdate(() => {
    if (loggedIn) {
      gsNavigate("/");
    }
  });
</script>

<Grid noGutter style="">
  <Row noGutter style="">
    <Column>
      {#if $store.api.loggedIn}
        Redirecting
      {:else}
        <div
          style=" border: 1px solid; max-width:25rem; background: white; margin: 0 auto; margin-top: 25vh;"
        >
          <FluidForm on:submit={handleLogin}>
            <Column style="text-align:left;">
              <h2
                style="margin-bottom: 20px; margin-left:15px; margin-top: 25px;"
              >
                Login
              </h2>
              <TextInput
                {invalid}
                labelText="User name"
                placeholder="Enter user name..."
                required
                bind:value={username}
                invalidText={invalidMessage}
              />
              <PasswordInput
                {invalid}
                required
                type="password"
                labelText="Password"
                placeholder="Enter password..."
                bind:value={password}
                invalidText={invalidMessage}
              />
              <ButtonSet style="align-items:right ">
                <Button
                  size="lg"
                  kind="secondary"
                  icon={Close}
                  style="width:100%"
                  disabled={!isEnabled}
                  on:click={onCancel}>Cancel</Button
                >
                <Column></Column>
                <Button
                  size="lg"
                  type="submit"
                  icon={ChevronRight}
                  style="width:100%">Submit</Button
                >
              </ButtonSet>
            </Column>
          </FluidForm>
        </div>
        <div class="text-center">
          <br />
          <DownloadCertificateLink />
        </div>
      {/if}
    </Column>
  </Row>
</Grid>
