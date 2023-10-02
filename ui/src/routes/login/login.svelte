<script lang="ts">
  import {
    Button,
    ButtonSet,
    Checkbox,
    Column,
    FluidForm,
    Grid,
    PasswordInput,
    Row,
    TextInput,
  } from "carbon-components-svelte";
  import { ChevronRight, Close } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { navigate } from "svelte-routing/src/history";
  import { afterUpdate } from "svelte";
  import { notificationstore } from "../../store/notifications";

  let username: string = localStorage.getItem("username") || "";
  let password: string = localStorage.getItem("password") || "";
  let rememberMe: boolean =
    (localStorage.getItem("rememberMe") || "") == "true";
  let isEnabled: boolean = true;
  let loggedIn: boolean = false;

  let invalidMessage: string = "";
  let invalid: boolean = false;
  let handleLogin = (e) => {
    e.preventDefault();
    var datatosend = { username: username, pass: password };
    $store.api.doCall("/auth/token", "post", datatosend).then(function (data) {
      if (data == undefined || data == null) {
        notificationstore.add({
          kind: "error",
          title: "Error:",
          subtitle: "Unable to get a correct response from the api",
          timeout: 30000,
        });
        return;
      } else if (data?.Validated) {
        localStorage.removeItem("jwt");
        localStorage.setItem("jwt", data.Jwtoken);
        store.loginSuccesful(data.Jwtoken);
      } else {
        invalidMessage = "Invalid username or password";
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

    if (rememberMe) {
      localStorage.setItem("username", username);
      localStorage.setItem("password", password);
      localStorage.setItem("rememberMe", "true");
    } else {
      localStorage.removeItem("username");
      localStorage.removeItem("password");
      localStorage.removeItem("rememberMe");
    }

    loggedIn = $store.api.loggedIn;
  }

  afterUpdate(() => {
    if (loggedIn) {
      navigate("/");
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
              <Checkbox
                id="remember-me"
                labelText="Remember me"
                style="margin:1em;"
                checked={rememberMe}
                on:change={() => {
                  rememberMe = !rememberMe;
                }}
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
      {/if}
    </Column>
  </Row>
</Grid>
