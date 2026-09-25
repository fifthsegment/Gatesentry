<script lang="ts">
  import {
    Button,
    InlineLoading,
    InlineNotification,
    Tag,
  } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import { onMount } from "svelte";

  import { store } from "../store/apistore";
  import ConnectedCertificateBasic from "./connectedCertificateBasic.svelte";
  import DownloadCertificateLink from "./downloadCertificateLink.svelte";

  interface CertificateInfo {
    name?: string;
    expiry?: string;
  }

  let info: CertificateInfo | null = null;
  let loading = true;
  let loadError = "";
  let editMode = false;

  onMount(async () => {
    try {
      info = await $store.api.doCall("/certificate/info");
    } catch (error) {
      loadError =
        error instanceof Error ? error.message : "Certificate details failed";
    } finally {
      loading = false;
    }
  });
</script>

<div class="certificate-heading">
  <div>
    <h3>{$_("Inspection certificate")}</h3>
    <p>
      {$_(
        "Install this certificate only on devices that you intend GateSentry to inspect.",
      )}
    </p>
  </div>
  <Button
    size="small"
    kind={editMode ? "secondary" : "tertiary"}
    on:click={() => (editMode = !editMode)}
  >
    {editMode ? $_("Hide advanced settings") : $_("Edit certificate")}
  </Button>
</div>

{#if loading}
  <InlineLoading description={$_("Loading certificate details…")} />
{:else if loadError}
  <InlineNotification
    lowContrast
    hideCloseButton
    kind="error"
    title={$_("Certificate unavailable")}
    subtitle={loadError}
  />
{:else if info}
  <dl class="certificate-summary">
    <div>
      <dt>{$_("Issuer name")}</dt>
      <dd>{info.name || $_("Unknown")}</dd>
    </div>
    <div>
      <dt>{$_("Expiry")}</dt>
      <dd>{info.expiry || $_("Unknown")}</dd>
    </div>
  </dl>
{/if}

<div class="certificate-download">
  <DownloadCertificateLink />
</div>

{#if editMode}
  <InlineNotification
    lowContrast
    hideCloseButton
    kind="warning"
    title={$_("Advanced certificate settings")}
    subtitle={$_(
      "Replacing certificate material can interrupt HTTPS inspection. Keep the private key secret and update the certificate and key as a matched pair.",
    )}
  />

  <div class="advanced-grid">
    <section aria-labelledby="certificate-pem-heading">
      <h4 id="certificate-pem-heading">{$_("Certificate (PEM)")}</h4>
      <p>
        {$_(
          "Public certificate installed on protected devices. It may be downloaded and shared with those devices.",
        )}
      </p>
      <ConnectedCertificateBasic
        settingName="capem"
        label={$_("HTTPS filtering certificate")}
      />
    </section>

    <section class="private-key" aria-labelledby="private-key-heading">
      <h4 id="private-key-heading">{$_("Private key (PEM)")}</h4>
      <p>
        {$_(
          "Sensitive signing key stored by GateSentry. Never install or share this value with client devices.",
        )}
      </p>
      <ConnectedCertificateBasic
        settingName="keypem"
        label={$_("HTTPS filtering private key")}
      />
    </section>
  </div>
{/if}

<style>
  .certificate-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }

  h3,
  h4,
  p {
    margin: 0;
  }

  .certificate-heading p,
  .advanced-grid p {
    max-width: 42rem;
    margin-top: 0.5rem;
    color: var(--cds-text-secondary, #525252);
    font-size: 0.875rem;
    line-height: 1.35;
  }

  .certificate-summary {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    margin: 1.25rem 0;
  }

  .certificate-summary div {
    min-width: 0;
    padding: 1rem;
    background: var(--cds-layer-02, #f4f4f4);
  }

  dt {
    color: var(--cds-text-secondary, #525252);
    font-size: 0.75rem;
  }

  dd {
    margin: 0.25rem 0 0;
    overflow-wrap: anywhere;
  }

  .certificate-download {
    margin-top: 1rem;
  }

  .advanced-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    margin-top: 1rem;
  }

  .advanced-grid section {
    min-width: 0;
    padding: 1rem;
    border: 1px solid var(--cds-border-subtle, #e0e0e0);
  }

  .private-key {
    border-top: 0.25rem solid var(--cds-support-error, #da1e28) !important;
  }

  @media (max-width: 42rem) {
    .certificate-heading {
      align-items: stretch;
      flex-direction: column;
    }

    .certificate-summary,
    .advanced-grid {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
