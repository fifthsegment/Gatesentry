<script lang="ts">
  import {
    Button,
    Checkbox,
    FluidForm,
    PasswordInput,
    TextInput,
  } from "carbon-components-svelte";
  import { ChevronRight, Close, Security } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { gsNavigate } from "../../lib/navigate";
  import { afterUpdate } from "svelte";
  import { notificationstore } from "../../store/notifications";
  import { createNotificationError } from "../../lib/utils";
  import { _ } from "svelte-i18n";
  import DownloadCertificateLink from "../../components/downloadCertificateLink.svelte";

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
        notificationstore.add(
          createNotificationError(
            { subtitle: $_("Unable to get a correct response from the api") },
            $_,
          ),
        );
        return;
      } else if (data?.Validated && data.Validated == "true") {
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
      gsNavigate("/");
    }
  });
</script>

<div class="login-page">
  {#if $store.api.loggedIn}
    <p>Redirecting…</p>
  {:else}
    <div class="login-card">
      <div class="login-header">
        <Security size={24} />
        <h2>GateSentry</h2>
      </div>
      <p class="login-subtitle">Sign in to your admin panel</p>

      <FluidForm on:submit={handleLogin}>
        <div class="login-fields">
          <TextInput
            {invalid}
            labelText="User name"
            placeholder="Enter user name…"
            required
            bind:value={username}
            invalidText={invalidMessage}
          />
          <PasswordInput
            {invalid}
            required
            type="password"
            labelText="Password"
            placeholder="Enter password…"
            bind:value={password}
            invalidText={invalidMessage}
          />
          <Checkbox
            id="remember-me"
            labelText="Remember me"
            checked={rememberMe}
            on:change={() => {
              rememberMe = !rememberMe;
            }}
          />
        </div>
        <div class="login-buttons">
          <Button
            kind="secondary"
            icon={Close}
            disabled={!isEnabled}
            on:click={onCancel}>Cancel</Button
          >
          <Button type="submit" icon={ChevronRight}>Submit</Button>
        </div>
      </FluidForm>
    </div>
    <div class="login-footer">
      <DownloadCertificateLink />
    </div>
  {/if}
</div>

<style>
  .login-page {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-start;
    padding: 15vh 1rem 2rem;
    min-height: 100vh;
    background: #f0f0f0;
  }

  .login-card {
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    max-width: 24rem;
    width: 100%;
    background: #fff;
    padding: 2rem 1.5rem 1.5rem;
  }

  .login-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }
  .login-header h2 {
    margin: 0;
    font-size: 1.5rem;
  }

  .login-subtitle {
    font-size: 0.875rem;
    color: #6f6f6f;
    margin: 0 0 1.5rem;
  }

  .login-fields {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .login-buttons {
    display: flex;
    gap: 0.75rem;
    margin-top: 1.5rem;
  }
  .login-buttons :global(.bx--btn) {
    flex: 1;
    max-width: none;
    justify-content: center;
  }

  .login-footer {
    margin-top: 1.25rem;
    text-align: center;
    font-size: 0.875rem;
  }

  @media (max-width: 671px) {
    .login-page {
      padding: 3rem 1rem 2rem;
    }
    .login-card {
      max-width: 100%;
      padding: 1.5rem 1rem 1.25rem;
      border-radius: 4px;
    }
    .login-header h2 {
      font-size: 1.25rem;
    }
  }
</style>
