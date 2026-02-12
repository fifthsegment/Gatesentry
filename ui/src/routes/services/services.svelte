<script lang="ts">
  import { Breadcrumb, BreadcrumbItem, Toggle } from "carbon-components-svelte";
  import { _ } from "svelte-i18n";
  import ToggleComponent from "../../components/toggle.svelte";
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Toggle Services</BreadcrumbItem>
</Breadcrumb>
<h2>Toggle Services</h2>

<br />

<div class="service-card">
  <ToggleComponent
    settingName="enable_dns_server"
    label="DNS Server"
    labelA="Not Running"
    labelB="Running"
  />
  <div class="info-notice">
    <span class="info-icon">ℹ</span>
    <p>
      Starts or stops the GateSentry DNS server completely. When stopped, DNS
      domain blocking is disabled and devices using GateSentry as their DNS
      server will lose DNS resolution. The proxy server and its content
      filtering rules (including per-user rules) continue to operate
      independently.
    </p>
  </div>
</div>

<div class="service-card">
  <ToggleComponent
    settingName="enable_dns_filtering"
    label="DNS Filtering"
    labelA="Disabled"
    labelB="Enabled"
  />
  <div class="info-notice">
    <span class="info-icon">ℹ</span>
    <p>
      Enables or disables DNS-level domain blocking. When disabled, the DNS
      server continues to run — all queries are resolved normally via the
      upstream DNS server, device discovery remains active, and DNS request
      statistics are still recorded (the Stats charts will continue to work).
      However, domains on the blocklist will no longer be blocked at the DNS
      level and no DNS block page will be shown. Proxy-level content filtering
      (URL blocking, HTTPS inspection, and per-user rules) is unaffected and
      continues to operate independently.
    </p>
  </div>
</div>

<style>
  .service-card {
    margin-bottom: 1.5rem;
  }

  .info-notice {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    background: #e3f2fd;
    border: 1px solid #90caf9;
    border-left: 4px solid #1976d2;
    border-radius: 4px;
    padding: 0.875rem 1rem;
    margin-top: 0.75rem;
  }

  .info-icon {
    color: #1565c0;
    font-weight: 700;
    font-size: 1rem;
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    border: 2px solid #1565c0;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-top: 1px;
  }

  .info-notice p {
    font-size: 0.8125rem;
    color: #1565c0;
    line-height: 1.5;
    margin: 0;
  }
</style>
