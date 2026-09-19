<script lang="ts">
  import {
    Button,
    Column,
    FluidForm,
    Grid,
    PasswordInput,
    Row,
    TextInput,
  } from "carbon-components-svelte";
  import {
    MAX_PASSWORD_BYTES,
    MIN_PASSWORD_BYTES,
    passwordHasValidByteLength,
    utf8ByteLength,
  } from "../../lib/credentials";
  import { getBasePath, gsNavigate } from "../../lib/navigate";

  let username = "";
  let password = "";
  let confirmation = "";
  let error = "";
  let submitting = false;
  let statusLoaded = false;
  let statusFailed = false;
  $: passwordBytes = utf8ByteLength(password);
  $: passwordLengthValid = passwordHasValidByteLength(password);
  // Single source of truth for why "Complete setup" is disabled. The button
  // state and the visible explanation are derived from the same value so the
  // form can never sit disabled without telling the operator what is missing.
  $: blockingReason = !username
    ? "Enter an administrator username."
    : password.length === 0
      ? "Enter a password."
      : !passwordLengthValid
        ? `Password must contain between ${MIN_PASSWORD_BYTES} and ${MAX_PASSWORD_BYTES} UTF-8 bytes (currently ${passwordBytes}).`
        : confirmation.length === 0
          ? "Re-enter the password to confirm it."
          : password !== confirmation
            ? "Passwords do not match."
            : "";

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

<Grid noGutter
  ><Row noGutter
    ><Column>
      <div class="setup-card">
        <FluidForm on:submit={submit}>
          <Column>
            <h2>Set up GateSentry</h2>
            <p>Create the administrator account for this installation.</p>
            {#if error}<p class="error" role="alert">{error}</p>{/if}
            {#if statusFailed}<Button
                type="button"
                kind="secondary"
                on:click={loadStatus}>Retry status check</Button
              >{/if}
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
              invalid={password.length > 0 && !passwordLengthValid}
              invalidText={`Password must contain between ${MIN_PASSWORD_BYTES} and ${MAX_PASSWORD_BYTES} UTF-8 bytes.`}
              bind:value={password}
            />
            <PasswordInput
              required
              autocomplete="new-password"
              labelText="Confirm password"
              invalid={confirmation.length > 0 && password !== confirmation}
              invalidText="Passwords do not match."
              bind:value={confirmation}
            />
            <br />
            {#if statusLoaded && blockingReason}
              <p class="hint" aria-live="polite">{blockingReason}</p>
            {/if}
            <Button
              type="submit"
              disabled={!statusLoaded || submitting || blockingReason !== ""}
              >Complete setup</Button
            >
          </Column>
        </FluidForm>
      </div>
    </Column></Row
  ></Grid
>

<style>
  .setup-card {
    border: 1px solid;
    max-width: 30rem;
    background: white;
    margin: 15vh auto 0;
    padding: 1rem;
  }
  h2,
  p {
    margin-bottom: 1rem;
  }
  .error {
    color: #da1e28;
  }
  .hint {
    color: #525252;
  }
</style>
