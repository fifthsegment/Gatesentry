<script lang="ts">
  import {
    Button,
    FluidForm,
    PasswordInput,
    TextInput,
  } from "carbon-components-svelte";
  import {
    MAX_PASSWORD_BYTES,
    MIN_PASSWORD_BYTES,
    passwordHasValidByteLength,
    utf8ByteLength,
  } from "../../lib/credentials";
  import { getBasePath, gsNavigate } from "../../lib/navigate";
  import AuthShell from "../../components/layout/AuthShell.svelte";

  let username = "";
  let password = "";
  let confirmation = "";
  let error = "";
  let submitting = false;
  let statusLoaded = false;
  let statusFailed = false;
  $: passwordBytes = utf8ByteLength(password);
  $: passwordLengthValid = passwordHasValidByteLength(password);
  $: passwordLengthError = password.length > 0 && !passwordLengthValid;
  $: passwordLengthMessage = `Password must contain between ${MIN_PASSWORD_BYTES} and ${MAX_PASSWORD_BYTES} UTF-8 bytes (currently ${passwordBytes}).`;
  $: confirmationMismatch = confirmation.length > 0 && password !== confirmation;
  // Single source of truth for why "Complete setup" is disabled. The button
  // state and the visible explanation are derived from the same value so the
  // form can never sit disabled without telling the operator what is missing.
  // Reasons that belong to one input are marked so they are rendered as that
  // field's own error text instead of being repeated below the form.
  $: blocker = !username
    ? { message: "Enter an administrator username.", field: "" }
    : password.length === 0
      ? { message: "Enter a password.", field: "" }
      : passwordLengthError
        ? { message: passwordLengthMessage, field: "password" }
        : confirmation.length === 0
          ? { message: "Re-enter the password to confirm it.", field: "" }
          : confirmationMismatch
            ? { message: "Passwords do not match.", field: "confirmation" }
            : { message: "", field: "" };
  $: blockingReason = blocker.message;
  $: hintMessage = blocker.field === "" ? blocker.message : "";

  async function loadStatus() {
    statusFailed = false;
    error = "";
    try {
      const response = await fetch(getBasePath() + "/api/setup/status");
      if (!response.ok) throw new Error("setup status failed");
      const status = await response.json();
      if (status.complete) {
        gsNavigate("/login");
        return;
      }
      statusLoaded = true;
    } catch (ignoredError) {
      statusFailed = true;
      error = "Unable to read setup status.";
    }
  }

  loadStatus();

  async function submit(event) {
    event.preventDefault();
    error = "";
    if (!passwordLengthValid) {
      error = `Password must contain between ${MIN_PASSWORD_BYTES} and ${MAX_PASSWORD_BYTES} UTF-8 bytes.`;
      return;
    }
    if (password !== confirmation) {
      error = "Passwords do not match.";
      return;
    }
    submitting = true;
    try {
      const response = await fetch(getBasePath() + "/api/setup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      if (!response.ok) {
        error =
          response.status === 409
            ? "Setup was already completed. Sign in instead."
            : "Setup could not be completed.";
        return;
      }
      password = "";
      confirmation = "";
      // Reload so App fetches the newly persisted setup state before applying
      // its route guard. This also clears every secret held by this component.
      window.location.assign(getBasePath() + "/login");
    } catch (ignoredError) {
      error = "Unable to contact GateSentry.";
    } finally {
      submitting = false;
    }
  }
</script>

<AuthShell
  title="Set up GateSentry"
  description="Create the administrator account for this installation."
>
  <FluidForm on:submit={submit}>
    {#if error}<p class="form-message form-message--error" role="alert">{error}</p>{/if}
    {#if statusFailed}
      <Button type="button" kind="secondary" on:click={loadStatus}>
        Retry status check
      </Button>
    {/if}
    <TextInput
      required
      autocomplete="username"
      labelText="Administrator username"
      bind:value={username}
    />
    <PasswordInput
      required
      autocomplete="new-password"
      labelText="Password"
      helperText={`Use ${MIN_PASSWORD_BYTES} to ${MAX_PASSWORD_BYTES} UTF-8 bytes.`}
      invalid={passwordLengthError}
      invalidText={passwordLengthMessage}
      bind:value={password}
    />
    <PasswordInput
      required
      autocomplete="new-password"
      labelText="Confirm password"
      invalid={confirmationMismatch}
      invalidText="Passwords do not match."
      bind:value={confirmation}
    />
    {#if statusLoaded && hintMessage}
      <p class="hint" aria-live="polite">{hintMessage}</p>
    {/if}
    <Button
      type="submit"
      disabled={!statusLoaded || submitting || blockingReason !== ""}
    >
      {submitting ? "Completing setup…" : "Complete setup"}
    </Button>
  </FluidForm>
</AuthShell>

<style>
  .form-message,
  .hint {
    margin-bottom: 1rem;
  }
  .form-message--error {
    color: #da1e28;
  }
  .hint {
    color: #525252;
  }
</style>
