<script lang="ts">
  import {
    Button,
    ButtonSet,
    FluidForm,
    PasswordInput,
    TextInput,
  } from "carbon-components-svelte";
  import { ChevronRight, Close } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { gsNavigate } from "../../lib/navigate";
  import { notificationstore } from "../../store/notifications";
  import { createNotificationError } from "../../lib/utils";
  import { _ } from "svelte-i18n";
  import { createEventDispatcher } from "svelte";
  import DownloadCertificateLink from "../../components/downloadCertificateLink.svelte";
  import AuthShell from "../../components/layout/AuthShell.svelte";

  let username = "";
  let password = "";
  let invalidMessage = "";
  let invalid = false;
  let submitting = false;
  const dispatch = createEventDispatcher<{ authenticated: void }>();

  localStorage.removeItem("password");
  localStorage.removeItem("rememberMe");

  const handleLogin = async (event: SubmitEvent) => {
    event.preventDefault();
    invalid = false;
    invalidMessage = "";
    submitting = true;
    try {
      const data = await $store.api.doCall("/auth/token", "post", {
        username,
        pass: password,
      });
      if (data?.Validated === true || data?.Validated === "true") {
        localStorage.removeItem("jwt");
        localStorage.setItem("jwt", data.Jwtoken);
        password = "";
        store.loginSuccesful(data.Jwtoken, data.Username || "");
        dispatch("authenticated");
        gsNavigate("/", { replace: true });
        return;
      }
      invalidMessage = $_("Invalid username or password");
      invalid = true;
    } catch {
      notificationstore.add(
        createNotificationError(
          { subtitle: $_("Unable to contact GateSentry. Try again.") },
          $_,
        ),
      );
    } finally {
      submitting = false;
    }
  };

  const cancel = () => {
    username = "";
    password = "";
    invalid = false;
    invalidMessage = "";
  };
</script>

<AuthShell
  title={$_("Sign in")}
  description={$_("Manage protection, policies, and connected devices.")}
>
  <FluidForm on:submit={handleLogin}>
    <TextInput
      {invalid}
      autocomplete="username"
      labelText={$_("User name")}
      placeholder={$_("Enter user name")}
      required
      bind:value={username}
      invalidText={invalidMessage}
    />
    <PasswordInput
      {invalid}
      autocomplete="current-password"
      required
      labelText={$_("Password")}
      placeholder={$_("Enter password")}
      bind:value={password}
      invalidText={invalidMessage}
    />
    <ButtonSet stacked>
      <Button
        size="lg"
        kind="secondary"
        icon={Close}
        type="button"
        disabled={submitting || (!username && !password)}
        on:click={cancel}
      >
        {$_("Clear")}
      </Button>
      <Button
        size="lg"
        type="submit"
        icon={ChevronRight}
        disabled={submitting || !username || !password}
      >
        {submitting ? $_("Signing in…") : $_("Sign in")}
      </Button>
    </ButtonSet>
  </FluidForm>

  <svelte:fragment slot="footer">
    <DownloadCertificateLink />
  </svelte:fragment>
</AuthShell>
