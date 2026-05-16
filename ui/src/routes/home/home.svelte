<script lang="ts">
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  import imageBin from "../../assets/front.jpg?inline";
  import { ChevronDown, Information } from "carbon-icons-svelte";
  import LaptopIcon from "../../components/icons/LaptopIcon.svelte";
  import MobileIcon from "../../components/icons/MobileIcon.svelte";
  import NetworkIcon from "../../components/icons/NetworkIcon.svelte";
  import Instructionsphone from "./instructionsphone.svelte";
  import Instructionscomputer from "./instructionscomputer.svelte";

  let loggedIn = false;
  $: loggedIn = $store.api.loggedIn;

  let dns_address = "";
  let dns_port = "";
  let proxy_port = "";
  let admin_port = "";

  $store.api.doCall("/status").then((json) => {
    if (json) {
      dns_address = json.dns_address || "";
      dns_port = json.dns_port || "53";
      proxy_port = json.proxy_port || "10413";
    }
  });

  $store.api
    .doCall("/wpad/info")
    .then((json) => {
      if (json) {
        admin_port = json.adminPort || "";
      }
    })
    .catch(() => {});

  let openSetup: string | null = null;

  function toggleSetup(id: string) {
    openSetup = openSetup === id ? null : id;
  }
</script>

<h2>{$_("Welcome to GateSentry")}</h2>
<p class="home-subtitle">
  {$_(
    "Use the menu to configure DNS filtering, proxy rules, and user policies.",
  )}
</p>

<!-- ── Status bar ── -->
{#if dns_address}
  <div class="status-bar">
    <NetworkIcon size={32} />
    <div class="status-items">
      <div class="status-item">
        <span class="status-label">{$_("Host")}</span>
        <strong>{dns_address}</strong>
      </div>
      <div class="status-item">
        <span class="status-label">{$_("DNS Port")}</span>
        <strong>{dns_port}</strong>
      </div>
      <div class="status-item">
        <span class="status-label">{$_("Proxy Port")}</span>
        <strong>{proxy_port}</strong>
      </div>
    </div>
  </div>
{:else}
  <div class="status-bar status-bar--loading">
    {$_("GateSentry is starting…")}
  </div>
{/if}

<!-- ── Setup Cards ── -->
<div class="setup-cards">
  <!-- Computer Setup -->
  <button
    class="setup-card"
    class:setup-card--open={openSetup === "computer"}
    on:click={() => toggleSetup("computer")}
  >
    <div class="setup-card-header">
      <LaptopIcon size={48} />
      <div class="setup-card-text">
        <h4>{$_("Set up your computer")}</h4>
        <p>
          {$_("Configure proxy and certificate for Windows, macOS, or Linux")}
        </p>
      </div>
      <ChevronDown
        size={20}
        style="transition:transform .2s;transform:rotate({openSetup ===
        'computer'
          ? 180
          : 0}deg);flex-shrink:0"
      />
    </div>
  </button>
  {#if openSetup === "computer"}
    <div class="setup-card-body">
      <Instructionscomputer
        proxyHost={dns_address}
        proxyPort={proxy_port}
        adminPort={admin_port}
      />
    </div>
  {/if}

  <!-- Phone Setup -->
  <button
    class="setup-card"
    class:setup-card--open={openSetup === "phone"}
    on:click={() => toggleSetup("phone")}
  >
    <div class="setup-card-header">
      <MobileIcon size={48} />
      <div class="setup-card-text">
        <h4>{$_("Set up your phone")}</h4>
        <p>{$_("Configure proxy and certificate for iOS or Android")}</p>
      </div>
      <ChevronDown
        size={20}
        style="transition:transform .2s;transform:rotate({openSetup === 'phone'
          ? 180
          : 0}deg);flex-shrink:0"
      />
    </div>
  </button>
  {#if openSetup === "phone"}
    <div class="setup-card-body">
      <Instructionsphone
        proxyHost={dns_address}
        proxyPort={proxy_port}
        adminPort={admin_port}
      />
    </div>
  {/if}
</div>

<!-- ── Info Section ── -->
<div class="home-grid">
  <div class="gs-card info-card">
    <div class="info-card-header">
      <Information size={20} />
      <h4>{$_("What is GateSentry?")}</h4>
    </div>
    <p>
      {$_(
        "GateSentry is a combined DNS and HTTP proxy server that filters malicious content and enforces browsing policies across your network. It blocks unwanted domains at the DNS level and inspects web traffic through the proxy for deeper content filtering.",
      )}
    </p>
  </div>

  <div class="gs-card info-card">
    <div class="info-card-header">
      <Information size={20} />
      <h4>{$_("Why MITM filtering?")}</h4>
    </div>
    <p>
      {$_(
        "Over 90% of web traffic is encrypted with HTTPS. Traditional network filters cannot inspect this traffic. Man-in-the-Middle (MITM) filtering decrypts, inspects, and re-encrypts traffic to detect malicious content and policy violations within encrypted connections, giving you visibility that DNS-only filtering cannot provide.",
      )}
    </p>
  </div>

  <div class="gs-card info-card diagram-card">
    <img
      src={imageBin}
      alt="GateSentry network filtering diagram"
      class="diagram-img"
    />
    <p class="diagram-caption">
      {$_(
        "Once configured, GateSentry filters traffic across all your devices.",
      )}
    </p>
  </div>
</div>

<style>
  .home-subtitle {
    color: #525252;
    margin-bottom: 1.25rem;
    font-size: 0.9375rem;
  }

  /* Status bar */
  .status-bar {
    display: flex;
    align-items: center;
    gap: 14px;
    background: #fff;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    padding: 12px 20px;
    margin-bottom: 1.5rem;
  }
  .status-bar--loading {
    color: #6f6f6f;
    font-size: 0.875rem;
  }
  .status-items {
    display: flex;
    gap: 1.5rem;
    flex-wrap: wrap;
  }
  .status-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.875rem;
  }
  .status-label {
    color: #6f6f6f;
  }

  /* Setup cards */
  .setup-cards {
    margin-bottom: 1.5rem;
  }
  .setup-card {
    display: block;
    width: 100%;
    background: #fff;
    border: 1px solid #e0e0e0;
    border-radius: 6px;
    padding: 16px 20px;
    margin-bottom: 8px;
    cursor: pointer;
    text-align: left;
    transition:
      border-color 0.15s,
      box-shadow 0.15s;
  }
  .setup-card:hover {
    border-color: #0f62fe;
  }
  .setup-card--open {
    border-color: #0f62fe;
    border-bottom-left-radius: 0;
    border-bottom-right-radius: 0;
    margin-bottom: 0;
  }
  .setup-card-header {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .setup-card-text {
    flex: 1;
  }
  .setup-card-text h4 {
    margin: 0;
    font-size: 1rem;
    font-weight: 600;
  }
  .setup-card-text p {
    margin: 2px 0 0 0;
    font-size: 0.8125rem;
    color: #6f6f6f;
  }

  .setup-card-body {
    background: #fff;
    border: 1px solid #0f62fe;
    border-top: none;
    border-bottom-left-radius: 6px;
    border-bottom-right-radius: 6px;
    padding: 20px;
    margin-bottom: 8px;
  }

  /* Info grid */
  .home-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 1.5rem;
  }
  .info-card {
    padding: 20px;
  }
  .info-card-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 0.75rem;
  }
  .info-card-header h4 {
    margin: 0;
    font-size: 0.9375rem;
    font-weight: 600;
  }
  .info-card p {
    font-size: 0.875rem;
    line-height: 1.65;
    color: #393939;
    margin: 0;
  }

  /* Diagram inside grid */
  .diagram-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
  }
  .diagram-img {
    width: 100%;
    max-width: 360px;
    border: 1px solid #d0d0d0;
    border-radius: 6px;
  }
  .diagram-caption {
    margin-top: 0.75rem;
    font-size: 0.8125rem;
    color: #6f6f6f;
  }

  @media (max-width: 960px) {
    .home-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 671px) {
    .status-items {
      gap: 0.75rem;
    }
    .home-grid {
      grid-template-columns: 1fr;
    }
    .setup-card-header {
      gap: 10px;
    }
    .setup-card-body {
      padding: 14px;
    }
  }
</style>
