<script lang="ts">
  import ConnectedCertificateBasic from "./connectedCertificateBasic.svelte";
  import { _ } from "svelte-i18n";
  import { getBasePath } from "../lib/navigate";

  import { store } from "../store/apistore";
  import { notificationstore } from "../store/notifications";
  import {
    createNotificationError,
    createNotificationSuccess,
  } from "../lib/utils";
  import { onMount } from "svelte";
  import { Edit, Renew, Download } from "carbon-icons-svelte";
  import {
    Button,
    Tile,
    InlineNotification,
    Modal,
    Tag,
  } from "carbon-components-svelte";

  interface CertInfoData {
    name: string;
    expiry: string;
    error: string;
  }

  let info: CertInfoData | null = null;
  let editMode = false;
  let generateModalOpen = false;
  let generating = false;

  let downloadLink = getBasePath() + `/api/files/certificate`;

  async function loadCertInfo() {
    info = await $store.api.doCall("/certificate/info");
  }

  onMount(async () => {
    await loadCertInfo();
  });

  async function generateNewCA() {
    generating = true;
    try {
      const result = await $store.api.doCall(
        "/certificate/generate",
        "post",
        {},
      );
      if (result && result.success) {
        info = result.info;
        notificationstore.add(
          createNotificationSuccess(
            {
              subtitle: $_("New CA certificate generated successfully"),
            },
            $_,
          ),
        );
      } else {
        notificationstore.add(
          createNotificationError(
            {
              subtitle:
                result?.error || $_("Failed to generate CA certificate"),
            },
            $_,
          ),
        );
      }
    } catch (e) {
      notificationstore.add(
        createNotificationError(
          { subtitle: $_("Failed to generate CA certificate") },
          $_,
        ),
      );
    } finally {
      generating = false;
      generateModalOpen = false;
      await loadCertInfo();
    }
  }

  function isExpiringSoon(expiry: string): boolean {
    if (!expiry) return false;
    const expiryDate = new Date(expiry);
    const now = new Date();
    const daysUntilExpiry =
      (expiryDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24);
    return daysUntilExpiry < 90;
  }

  function isExpired(expiry: string): boolean {
    if (!expiry) return false;
    return new Date(expiry) < new Date();
  }
</script>

<div class="cert-section">
  {#if info === null}
    <!-- Skeleton placeholder while loading -->
    <Tile class="cert-info-tile">
      <div class="cert-info-grid">
        <div class="cert-info-item">
          <span class="cert-info-label">{$_("Issuer (CN)")}</span>
          <span class="cert-skeleton-line" style="width:60%">&nbsp;</span>
        </div>
        <div class="cert-info-item">
          <span class="cert-info-label">{$_("Expires")}</span>
          <span class="cert-skeleton-line" style="width:45%">&nbsp;</span>
        </div>
      </div>
    </Tile>
  {:else if info.error}
    <InlineNotification
      kind="error"
      title={$_("Certificate Error")}
      subtitle={info.error}
      hideCloseButton
    />
  {:else}
    <Tile class="cert-info-tile">
      <div class="cert-info-grid">
        <div class="cert-info-item">
          <span class="cert-info-label">{$_("Issuer (CN)")}</span>
          <span class="cert-info-value">{info.name}</span>
        </div>
        <div class="cert-info-item">
          <span class="cert-info-label">{$_("Expires")}</span>
          <span class="cert-info-value">
            {info.expiry}
            {#if isExpired(info.expiry)}
              <Tag type="red" size="sm">{$_("Expired")}</Tag>
            {:else if isExpiringSoon(info.expiry)}
              <Tag type="magenta" size="sm">{$_("Expiring Soon")}</Tag>
            {:else}
              <Tag type="green" size="sm">{$_("Valid")}</Tag>
            {/if}
          </span>
        </div>
      </div>
    </Tile>
  {/if}

  <div class="cert-actions">
    <Button
      size="small"
      kind="primary"
      icon={Renew}
      disabled={generating}
      on:click={() => (generateModalOpen = true)}
    >
      {$_("Generate New CA")}
    </Button>

    <a href={downloadLink} target="_blank" class="download-btn">
      <Button size="small" kind="secondary" icon={Download}>
        {$_("Download CA Certificate")}
      </Button>
    </a>

    <Button
      size="small"
      kind="ghost"
      icon={Edit}
      on:click={() => (editMode = !editMode)}
    >
      {editMode ? $_("Hide Editor") : $_("Upload Custom CA")}
    </Button>
  </div>

  <InlineNotification
    kind="info"
    lowContrast
    title={$_("Client Setup Required")}
    hideCloseButton
  >
    <div slot="subtitle">
      <p>
        {$_(
          "After generating or uploading a CA certificate, download the .crt file and install it as a Trusted Root CA on each device that uses this proxy.",
        )}
      </p>
      <br />
      <p>
        <strong>Windows:</strong>
        {$_(
          "Double-click gatesentry-ca.crt → Install Certificate → Local Machine → Place all certificates in the following store → Trusted Root Certification Authorities → Finish.",
        )}
      </p>
      <p>
        <strong>macOS:</strong>
        {$_(
          "Double-click gatesentry-ca.crt to open Keychain Access → select the 'System' keychain → find the certificate, double-click it → expand Trust → set 'When using this certificate' to Always Trust → close and enter your password.",
        )}
      </p>
      <p>
        <strong>Linux:</strong>
        {$_(
          "Copy gatesentry-ca.crt to /usr/local/share/ca-certificates/ and run: sudo update-ca-certificates. For Firefox/Chrome, import via browser settings → Privacy & Security → Certificates → Import.",
        )}
      </p>
    </div>
  </InlineNotification>

  {#if editMode}
    <div class="cert-editor">
      <ConnectedCertificateBasic
        settingName="capem"
        label={$_("CA Certificate (PEM)")}
      />

      <ConnectedCertificateBasic
        settingName="keypem"
        label={$_("CA Private Key (PEM)")}
      />
    </div>
  {/if}
</div>

<Modal
  bind:open={generateModalOpen}
  modalHeading={$_("Generate New CA Certificate")}
  primaryButtonText={generating ? $_("Generating...") : $_("Generate")}
  secondaryButtonText={$_("Cancel")}
  primaryButtonDisabled={generating}
  on:click:button--secondary={() => (generateModalOpen = false)}
  on:submit={generateNewCA}
>
  <p>
    {$_(
      "This will generate a new 4096-bit RSA CA certificate valid for 10 years. The new certificate will replace the current one immediately.",
    )}
  </p>
  <br />
  <p>
    <strong
      >{$_(
        "Important: After generating a new CA, you must re-download and re-install the certificate on all client devices that use this proxy.",
      )}</strong
    >
  </p>
  <br />
  <p>
    {$_(
      "Any existing HTTPS connections through the proxy will need to be re-established.",
    )}
  </p>
</Modal>

<style>
  .cert-section {
    margin-top: 1rem;
  }
  :global(.cert-info-tile) {
    margin-bottom: 1rem;
  }
  .cert-info-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .cert-info-label {
    display: block;
    font-size: 0.75rem;
    color: #525252;
    margin-bottom: 0.25rem;
  }
  .cert-info-value {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.875rem;
    font-weight: 500;
  }

  .cert-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
  }
  .download-btn {
    text-decoration: none;
  }
  .cert-editor {
    margin-top: 1rem;
    padding: 1rem;
    border: 1px solid #e0e0e0;
    background: #f4f4f4;
  }

  .cert-skeleton-line {
    display: block;
    height: 1.125rem;
    border-radius: 3px;
    background: linear-gradient(90deg, #e0e0e0 25%, #ececec 50%, #e0e0e0 75%);
    background-size: 200% 100%;
    animation: shimmer 1.5s ease-in-out infinite;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }

  @media (max-width: 671px) {
    .cert-info-grid {
      grid-template-columns: 1fr;
    }
    .cert-actions {
      flex-direction: column;
      align-items: stretch;
    }
    .cert-actions :global(.bx--btn) {
      max-width: 100%;
      width: 100%;
      justify-content: center;
    }
    .download-btn {
      display: block;
    }
  }
</style>
