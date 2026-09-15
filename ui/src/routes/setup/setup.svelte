<script lang="ts">
  import { Button, Column, FluidForm, Grid, PasswordInput, Row, TextInput } from "carbon-components-svelte";
  import { getBasePath, gsNavigate } from "../../lib/navigate";

  let username = "";
  let password = "";
  let confirmation = "";
  let authorization = "";
  let requiresAuthorization = false;
  let error = "";
  let submitting = false;
  let statusLoaded = false;
  let statusFailed = false;

  async function loadStatus() {
    statusFailed = false;
    error = "";
    try {
      const response = await fetch(getBasePath() + "/api/setup/status");
      if (!response.ok) throw new Error("setup status failed");
      const status = await response.json();
      if (status.complete) { gsNavigate("/login"); return; }
      requiresAuthorization = status.requires_authorization;
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
    if (password !== confirmation) { error = "Passwords do not match."; return; }
    submitting = true;
    try {
      const response = await fetch(getBasePath() + "/api/setup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password, authorization }),
      });
      if (!response.ok) { error = response.status === 409 ? "Setup was already completed. Sign in instead." : "Setup could not be completed."; return; }
      password = ""; confirmation = ""; authorization = "";
      // Reload so App fetches the newly persisted setup state before applying
      // its route guard. This also clears every secret held by this component.
      window.location.assign(getBasePath() + "/login");
    } catch (ignoredError) { error = "Unable to contact GateSentry."; }
    finally { submitting = false; }
  }
</script>

<Grid noGutter><Row noGutter><Column>
  <div class="setup-card">
    <FluidForm on:submit={submit}>
      <Column>
        <h2>Set up GateSentry</h2>
        <p>Create the administrator account for this installation.</p>
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        {#if statusFailed}<Button type="button" kind="secondary" on:click={loadStatus}>Retry status check</Button>{/if}
        <TextInput required autocomplete="username" labelText="Administrator username" bind:value={username} />
        <PasswordInput required autocomplete="new-password" maxlength={72} labelText="Password" helperText="Use 12 to 72 characters." bind:value={password} />
        <PasswordInput required autocomplete="new-password" maxlength={72} labelText="Confirm password" bind:value={confirmation} />
        {#if requiresAuthorization}
          <PasswordInput required autocomplete="off" labelText="Bootstrap authorization" bind:value={authorization} />
        {/if}
        <Button type="submit" disabled={!statusLoaded || submitting || !username || password.length < 12 || password.length > 72 || password !== confirmation}>Complete setup</Button>
      </Column>
    </FluidForm>
  </div>
</Column></Row></Grid>

<style>
  .setup-card { border: 1px solid; max-width: 30rem; background: white; margin: 15vh auto 0; padding: 1rem; }
  h2, p { margin-bottom: 1rem; }
  .error { color: #da1e28; }
</style>
