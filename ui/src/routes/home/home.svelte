<script lang="ts">
  import { onMount } from "svelte";
  import { Column, Row, Button, Tag } from "carbon-components-svelte";
  import { Restart } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";

  type CheckStep = { state: string; action?: string; detail?: string };
  type Progress = {
    started_at?: string;
    first_explained_protection_at?: string;
    };
  type ProtectionCheck = {
    at: string;
    domain: string;
    decision: string;
    rcode: string;
    cname?: string;
    upstream: string;
    error?: string;
  };
  type OnboardingStatus = {
    dns_running: boolean;
    listen_addr: string;
    listen_port: string;
    upstream: string;
    blocked_domains: number;
    device_count: number;
    bind: CheckStep;
    resolver: CheckStep;
    blocklist: CheckStep;
    progress: Progress;
  };

  let status: OnboardingStatus | null = null;
  let statusError = "";
  let check: ProtectionCheck | null = null;
  let checkState = "";
  let checkDetail = "";
  let checkAction = "";
  let checking = false;

  const loadStatus = async () => {
    statusError = "";
    try {
      status = await $store.api.doCall("/onboarding/status");
    } catch (error) {
      statusError = "Unable to load onboarding status.";
    }
  };

  const runProtectionCheck = async () => {
    checking = true;
    checkDetail = "";
    checkAction = "";
    checkState = "";
    try {
      const response = await $store.api.doCall(
        "/onboarding/protection-check",
        "post",
        {},
      );
      check = response.check;
      checkState = response.state;
      checkDetail = response.detail;
      checkAction = response.action || "";
      await loadStatus();
    } catch (error) {
      checkState = "failed";
      checkDetail = "The protection check did not complete.";
      checkAction =
        "Verify the DNS listener and upstream resolver, then retry the check.";
    } finally {
      checking = false;
    }
  };

  const checkTag = (state: string) => {
    if (state === "ok") return "green";
    if (state === "failed") return "red";
    return "gray";
  };

  const stateLabel = (state: string) => {
    if (state === "ok") return $_("Ready");
    if (state === "failed") return $_("Needs attention");
    return $_("Not verified yet");
  };

  const elapsedMinutes = () => {
    if (!status?.progress?.started_at) return null;
    const started = new Date(status.progress.started_at).getTime();
    const protectedAt = status.progress.first_explained_protection_at
      ? new Date(status.progress.first_explained_protection_at).getTime()
      : Date.now();
    if (Number.isNaN(started) || Number.isNaN(protectedAt)) return null;
    return Math.max(0, Math.round((protectedAt - started) / 60000));
  };

  onMount(loadStatus);
</script>

<h2>{$_("Get protected in five guided steps")}</h2>
<p>
  {$_(
    "This page walks through DNS-first setup: pick a resolver, load blocklists, point a device or router at GateSentry, wait for the first device, then run the protection check.",
  )}
</p>

{#if statusError}
  <div class="simple-border" role="alert">{statusError}</div>
  <Button kind="secondary" on:click={loadStatus}>
    {$_("Retry status check")}
  </Button>
{/if}

{#if status}
  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>
          1. {$_("Resolver and listener")}
          <Tag type={checkTag(status.resolver.state)}>
            {stateLabel(status.resolver.state)}
          </Tag>
        </h4>
        <p>
          {$_("Upstream resolver")}: <strong>{status.upstream}</strong><br />
          {$_("DNS listener")}:
          <strong>{status.listen_addr}:{status.listen_port}</strong>
        </p>
        <p class="detail">{status.resolver.detail}</p>
        {#if status.resolver.action}
          <p class="action">{status.resolver.action}</p>
        {/if}
        <p class="hint">
          {$_(
            "Change the resolver from DNS Server settings if this upstream is unreachable.",
          )}
        </p>
        {#if status.bind.state !== "ok"}
          <p class="action">{status.bind.detail} {status.bind.action}</p>
        {/if}
      </div>
    </Column>
  </Row>

  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>
          2. {$_("Blocklist readiness")}
          <Tag type={checkTag(status.blocklist.state)}>
            {stateLabel(status.blocklist.state)}
          </Tag>
        </h4>
        <p>
          {$_("Blocked domains loaded")}:
          <strong>{status.blocked_domains}</strong>
        </p>
        <p class="detail">{status.blocklist.detail}</p>
        {#if status.blocklist.action}
          <p class="action">{status.blocklist.action}</p>
        {/if}
        <p class="hint">
          {$_(
            "DNS blocking acts on domains. It cannot inspect or explain URL, MIME, keyword, or image-content decisions.",
          )}
        </p>
      </div>
    </Column>
  </Row>

  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>3. {$_("Point clients or your router at GateSentry")}</h4>
        <p>
          {$_(
            "Set the DNS server on a device, or the DHCP/DNS setting on your router, to the GateSentry host address.",
          )}
        </p>
        <pre>
  dig @&lt;gatesentry-host-ip&gt; example.com</pre>
        <p class="hint">
          {$_(
            "Devices that use GateSentry for DNS are the only devices covered. Devices that use a different resolver are unverified.",
          )}
        </p>
      </div>
    </Column>
  </Row>

  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>
          4. {$_("First device")}
          <Tag type={status.device_count > 0 ? "green" : "red"}>
            {status.device_count > 0
              ? $_("Discovered")
              : $_("Waiting for a device")}
          </Tag>
        </h4>
        <p>
          {$_("Discovered devices")}: <strong>{status.device_count}</strong>
        </p>
        <p class="hint">
          {$_(
            "Point a device at GateSentry, browse briefly, then refresh. Devices are verified only when GateSentry observes their DNS traffic.",
          )}
        </p>
        <Button kind="secondary" icon={Restart} on:click={loadStatus}>
          {$_("Refresh devices")}
        </Button>
      </div>
    </Column>
  </Row>

  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>5. {$_("Verify actual protection")}</h4>
        <p>
          {$_(
            "Startup is not protection. This check asks the running GateSentry DNS server to resolve a controlled test domain and records the decision it made.",
          )}
        </p>
        <Button
          on:click={runProtectionCheck}
          disabled={checking || !status.dns_running}
        >
          {checking ? $_("Checking...") : $_("Run protection check")}
        </Button>
        {#if checkState}
          <div class="check-result" role="status">
            <Tag type={checkTag(checkState)}>
              {checkState === "ok" ? $_("Protected") : $_("Not protected yet")}
            </Tag>
            <p class="detail">{checkDetail}</p>
            {#if check}
              <p>
                {$_("Test domain")}: <code>{check.domain}</code><br />
                {$_("Answer")}: <code>{check.rcode}</code>
                {#if check.cname}
                  <br />{$_("Explanation")}:
                  <code>{check.cname}</code>
                {/if}<br />
                {$_("Upstream")}:
                <code>{check.upstream}</code>
              </p>
            {/if}
            {#if checkAction}<p class="action">{checkAction}</p>{/if}
          </div>
        {/if}
        {#if status.progress.first_explained_protection_at}
          <p>
            <strong>
              {$_("First explained protection recorded")}:
              {new Date(
                status.progress.first_explained_protection_at,
              ).toLocaleString()}
            </strong>
          </p>
          {#if elapsedMinutes() !== null}
            <p>
              {$_("Time from setup start to first explained protection")}:
              {elapsedMinutes()}
              {$_("minutes")} ({$_("target")} ≤ 10).
            </p>
          {/if}
        {/if}
      </div>
    </Column>
  </Row>

  <Row>
    <Column>
      <div class="simple-border onboarding-card">
        <h4>{$_("Optional: HTTPS inspection")}</h4>
        <p>
          {$_(
            "DNS filtering needs no certificates and is the simple default. HTTPS inspection is advanced and opt-in: it enables URL, MIME, keyword, and content filtering by trusting the GateSentry CA certificate.",
          )}
        </p>
        <p class="hint">
          {$_(
            "It is not required for the steps above. Devices that have not installed the certificate, and traffic that bypasses the proxy, remain outside HTTPS inspection.",
          )}
        </p>
        <p class="hint">
          {$_(
            "Enable it from the dashboard after reviewing the certificate trust and exclusion guidance.",
          )}
        </p>
      </div>
    </Column>
  </Row>
{/if}

<style>
  .onboarding-card {
    margin-top: 1rem;
    padding: 1rem;
    line-height: 1.6;
  }
  .detail {
    color: #525252;
  }
  .action {
    color: #da1e28;
  }
  .hint {
    color: #6f6f6f;
  }
  .check-result {
    margin-top: 1rem;
  }
  pre {
    background: #f4f4f4;
    padding: 0.5rem;
  }
</style>
