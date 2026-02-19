<script lang="ts">
  import { _ } from "svelte-i18n";

  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
  import ConnectedCertificateComposed from "../../components/connectedCertificateComposed.svelte";
  import WpadSettings from "../../components/wpadSettings.svelte";
  import {
    Settings as SettingsIcon,
    Report,
    Security,
    Network_3,
  } from "carbon-icons-svelte";
</script>

<div class="gs-page-title">
  <SettingsIcon size={24} />
  <h2>{$_("Settings")}</h2>
</div>

<!-- ── Logging & Administration ── -->
<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <Report size={20} />
      <h5>{$_("Logging & Administration")}</h5>
    </div>

    <div class="card-fields">
      <ConnectedGeneralSettingInput
        keyName="log_location"
        title={$_("Log Location")}
        labelText={$_("Log Location")}
        type="text"
        helperText={$_("Directory where GateSentry writes log files")}
      />

      <ConnectedGeneralSettingInput
        keyName="admin_username"
        helperText={$_("The administrator account used to sign in to this UI")}
        type="text"
        title={$_("Admin Username")}
        labelText={$_("Admin Username")}
        disabled={true}
      />
    </div>
  </div>
</section>

<!-- ── MITM Filtering & Certificate Authority ── -->
<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <Security size={20} />
      <h5>{$_("MITM Filtering & Certificate Authority")}</h5>
    </div>

    <HttpsToggle />

    <ConnectedCertificateComposed />
  </div>
</section>

<!-- ── WPAD / Proxy Auto-Discovery ── -->
<section class="gs-section">
  <div class="gs-card settings-card">
    <div class="card-header">
      <Network_3 size={20} />
      <h5>{$_("Automatic Proxy Discovery (WPAD)")}</h5>
    </div>

    <WpadSettings />
  </div>
</section>

<style>
  .card-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }
  .card-header h5 {
    margin: 0;
    font-weight: 600;
  }
  .card-fields {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  /* full-width notifications inside settings cards */
  .settings-card :global(.bx--inline-notification) {
    max-width: 100%;
  }

  /* remove extra tile padding/borders since we're already in a card */
  .settings-card :global(.bx--tile) {
    border: 1px solid #e0e0e0;
  }
</style>
