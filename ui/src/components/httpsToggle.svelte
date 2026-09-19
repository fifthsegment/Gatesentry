<script lang="ts">
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";
  import {
    ComposedModal,
    ModalBody,
    ModalFooter,
    ModalHeader,
    Accordion,
    AccordionItem,
    InlineNotification,
    Loading,
    Toggle,
  } from "carbon-components-svelte";
  import Privacynotice from "./privacynotice.svelte";
  import { store } from "../store/apistore";

  const SETTING = "enable_https_filtering";

  // The settings store owns this value. `toggled` mirrors the control and
  // `storedValue` mirrors what the gateway has persisted, so a failed save
  // never leaves the control claiming a state the gateway does not have.
  let loaded = false;
  let saving = false;
  let toggled = false;
  let storedValue = false;
  let error = "";

  // Enabling inspection is gated on explicit consent; disabling is not.
  let modalOpen = false;
  let consentAccepted = false;

  onMount(loadSetting);

  async function loadSetting() {
    try {
      const json = await $store.api.doCall("/settings/" + SETTING);
      storedValue = json.Value == "true";
      toggled = storedValue;
      loaded = true;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  async function persist(value: boolean) {
    saving = true;
    error = "";
    try {
      await $store.api.doCall("/settings/" + SETTING, "post", {
        key: SETTING,
        value: value ? "true" : "false",
      });
      storedValue = value;
      toggled = value;
    } catch (e) {
      toggled = storedValue;
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }

  // The consent gate: nothing is persisted until the operator accepts the
  // statement and confirms in the modal. Carbon's Toggle reports every value
  // change, including the ones this component makes while reverting, so the
  // handler is written to be idempotent for a value that already matches the
  // gateway state.
  function handleUserToggle(value: boolean) {
    toggled = value;
    if (value === storedValue) {
      return;
    }
    if (!value) {
      persist(false);
      return;
    }
    toggled = false;
    consentAccepted = false;
    modalOpen = true;
  }

  async function enableInspection() {
    if (!consentAccepted) {
      return;
    }
    modalOpen = false;
    consentAccepted = false;
    await persist(true);
  }

  function cancelModal() {
    modalOpen = false;
    consentAccepted = false;
    toggled = storedValue;
  }
</script>

<div style="margin-top: 15px;">
  {#if !loaded}
    <Loading />
  {:else}
    <Toggle
      labelText={$_("HTTPS Filtering - Man In The Middle Filtering")}
      labelA={$_("Disabled")}
      labelB={$_("Enabled")}
      toggled={toggled}
      disabled={saving}
      on:toggle={(event) => handleUserToggle(event.detail.toggled)}
    />
  {/if}

  {#if error}
    <InlineNotification kind="error" title={$_("Error")} subtitle={error} on:close={() => (error = "")} />
  {/if}

  <ComposedModal
    open={modalOpen}
    preventCloseOnClickOutside
    on:close={cancelModal}
    on:click:button--primary={enableInspection}
  >
    <ModalHeader label={$_("Consent")} title={$_("HTTPS Filtering - Man In The Middle Filtering")} />
    <ModalBody hasForm>
      <Privacynotice />

      <Accordion>
        <AccordionItem title={$_("Platform certificate instructions")}>
          <p>
            Download the CA certificate first, then install it in the client
            trust store. Full steps for every platform are in
            <a href="https://github.com/fifthsegment/Gatesentry/blob/master/docs/https-inspection.md" target="_blank" rel="noopener noreferrer">docs/https-inspection.md</a>.
          </p>
          <ul>
            <li><b>iOS / iPadOS:</b> Settings, Profile Downloaded, Install, Certificate Trust Settings, enable full trust.</li>
            <li><b>macOS:</b> Keychain Access, drag to System keychain, double-click, Trust, Always Trust.</li>
            <li><b>Windows:</b> Right-click .pem, Install Certificate, Local Machine, Trusted Root Certification Authorities.</li>
            <li><b>Android:</b> Settings, Security, Install a certificate, CA certificate.</li>
            <li><b>Linux system store:</b> copy to <code>/usr/local/share/ca-certificates/</code> and run <code>update-ca-certificates</code>.</li>
            <li><b>Firefox:</b> Settings, Privacy and Security, Certificates, View Certificates, Authorities, Import (separate trust store).</li>
          </ul>
        </AccordionItem>
        <AccordionItem title={$_("Verification")}>
          <p>
            After installing the CA and configuring the proxy, browse to an HTTPS
            site that is not on the exclusion list. Inspect the certificate chain:
            the leaf certificate issuer must be the GateSentry CA. Then check the
            HTTPS Inspection Status panel below: the inspect count should
            increase. For a site on the exclusion list, the chain should show the
            real issuer and the bypass count should increase. This is a step you
            perform, not a claim that interception works.
          </p>
        </AccordionItem>
      </Accordion>

      <div style="margin-top: 15px;">
        <label>
          <input type="checkbox" bind:checked={consentAccepted} />
          {$_("I accept the consent statement above")}
        </label>
      </div>
      {#if !consentAccepted}
        <InlineNotification kind="info" title={$_("Consent")} subtitle={$_("You must accept the consent statement to enable inspection.")} />
      {/if}
    </ModalBody>
    <ModalFooter
      secondaryButtonText={$_("Cancel")}
      on:click:button--secondary={cancelModal}
      primaryButtonText={$_("Accept consent and enable inspection")}
      primaryButtonDisabled={!consentAccepted}
    />
  </ComposedModal>
</div>
