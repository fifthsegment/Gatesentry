<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import {
    Button,
    CodeSnippet,
    InlineNotification,
    ProgressIndicator,
    ProgressStep,
    StructuredList,
    StructuredListBody,
    StructuredListCell,
    StructuredListRow,
    Tag,
  } from "carbon-components-svelte";
  import { ArrowRight, Restart } from "carbon-icons-svelte";
  import { store } from "../../store/apistore";
  import { gsNavigate } from "../../lib/navigate";
  import { _ } from "svelte-i18n";

  type CheckStep = { state: string; action?: string; detail?: string };
  type OnboardingStep = { name: string; state: string };
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
    steps: OnboardingStep[];
    progress: Progress;
  };
  type Count = { key: string; count: number };
  type Summary = {
    total: number;
    by_action: Record<string, number>;
    unique_clients: number;
    top_blocked_domains: Count[];
  };
  type Notice = { key: string; title: string; subtitle: string };

  // "clients" is an instruction and "https_inspection" is optional, so only
  // these steps carry a state setup can reach.
  const measuredSteps = [
    "resolver",
    "blocklist",
    "first_device",
    "protection_check",
  ];

  let status: OnboardingStatus | null = null;
  let summary: Summary | null = null;
  let httpsOn: boolean | null = null;
  let statusError = "";
  let check: ProtectionCheck | null = null;
  let checkState = "";
  let checkDetail = "";
  let checkAction = "";
  let checking = false;
  let timer: ReturnType<typeof setInterval> | null = null;

  const loadStatus = async () => {
    statusError = "";
    try {
      status = await $store.api.doCall("/onboarding/status");
    } catch (error) {
      statusError = "Unable to load the gateway status.";
    }
  };

  const loadSummary = async () => {
    try {
      summary = await $store.api.doCall("/decisions/summary?days=1");
    } catch (error) {
      summary = null;
    }
  };

  const loadHttps = async () => {
    try {
      const res = await $store.api.doCall("/settings/enable_https_filtering");
      httpsOn = res?.Value === "true";
    } catch (error) {
      httpsOn = null;
    }
  };

  const refresh = () => Promise.all([loadStatus(), loadSummary(), loadHttps()]);

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

  const stepLabel = (name: string) => {
    if (name === "resolver") return $_("Resolver and listener");
    if (name === "blocklist") return $_("Blocklists loaded");
    if (name === "first_device") return $_("First device discovered");
    return $_("Protection verified");
  };

  const stepDetail = (name: string) => {
    if (!status) return "";
    if (name === "resolver") return status.resolver.detail || "";
    if (name === "blocklist") return status.blocklist.detail || "";
    if (name === "first_device") {
      return `${status.device_count} ${$_("devices discovered")}`;
    }
    if (status.progress.first_explained_protection_at) {
      return `${$_("Verified")} ${new Date(
        status.progress.first_explained_protection_at,
      ).toLocaleString()}`;
    }
    return $_("Not verified yet");
  };

  const collectNotices = (current: OnboardingStatus): Notice[] => {
    const notices: Notice[] = [];
    if (current.bind.state !== "ok") {
      notices.push({
        key: "bind",
        title: $_("DNS listener is not ready"),
        subtitle: [current.bind.detail, current.bind.action]
          .filter(Boolean)
          .join(" "),
      });
    }
    if (current.resolver.state !== "ok" && current.resolver.action) {
      notices.push({
        key: "resolver",
        title: $_("Upstream resolver is not reachable"),
        subtitle: current.resolver.action,
      });
    }
    if (current.blocklist.state !== "ok" && current.blocklist.action) {
      notices.push({
        key: "blocklist",
        title: $_("Blocklist empty"),
        subtitle: current.blocklist.action,
      });
    }
    return notices;
  };

  // A loopback or wildcard address is not reachable from other devices.
  const unreachableHosts = ["", "0.0.0.0", "::", "[::]", "127.0.0.1", "::1"];
  const dnsHost = () => {
    const addr = status?.listen_addr || "";
    return unreachableHosts.includes(addr) ? "<gatesentry-host-ip>" : addr;
  };

  const fmt = (n: number | undefined) =>
    typeof n === "number" ? n.toLocaleString() : "—";

  $: readiness = (status?.steps || []).filter((step) =>
    measuredSteps.includes(step.name),
  );
  $: pendingIndex = readiness.findIndex((step) => step.state !== "ok");
  $: setupDone = readiness.length > 0 && pendingIndex === -1;
  $: notices = status ? collectNotices(status) : [];
  $: protectedYet = Boolean(status?.progress.first_explained_protection_at);
  $: blocked24 = summary?.by_action?.block || 0;
  $: blockRate =
    summary && summary.total > 0
      ? ((blocked24 / summary.total) * 100).toFixed(1) + "%"
      : "—";

  onMount(() => {
    refresh();
    timer = setInterval(loadSummary, 30000);
  });
  onDestroy(() => {
    if (timer) clearInterval(timer);
  });
</script>

<div class="page">
  <header class="page-head">
    <div>
      <h2>{$_("Overview")}</h2>
      <p class="lede">
        {$_(
          "GateSentry is a self-hosted internet safety gateway. Devices that use it for DNS, or send traffic through its proxy, are filtered by your policies.",
        )}
      </p>
    </div>
    <Button kind="ghost" size="small" icon={Restart} on:click={refresh}>
      {$_("Refresh")}
    </Button>
  </header>

  {#if statusError}
    <InlineNotification
      kind="error"
      title={$_("Status unavailable")}
      subtitle={statusError}
      on:close={() => (statusError = "")}
    />
  {/if}

  {#if status}
    {#each notices as notice (notice.key)}
      <InlineNotification
        kind="error"
        lowContrast
        hideCloseButton
        title={notice.title}
        subtitle={notice.subtitle}
      />
    {/each}

    <section class="tiles" aria-label={$_("Gateway status")}>
      <div class="tile">
        <span class="tile-label">{$_("Protection")}</span>
        <Tag type={protectedYet ? "green" : "red"}>
          {protectedYet ? $_("Protected") : $_("Not verified")}
        </Tag>
        <span class="tile-foot">
          {status.dns_running ? $_("DNS server running") : $_("DNS server stopped")}
        </span>
      </div>
      <div class="tile">
        <span class="tile-label">{$_("Devices")}</span>
        <span class="tile-value">{fmt(status.device_count)}</span>
        <span class="tile-foot">{$_("discovered on the network")}</span>
      </div>
      <div class="tile">
        <span class="tile-label">{$_("Blocklist")}</span>
        <span class="tile-value">{fmt(status.blocked_domains)}</span>
        <span class="tile-foot">{$_("domains loaded")}</span>
      </div>
      <div class="tile">
        <span class="tile-label">{$_("HTTPS inspection")}</span>
        <Tag type={httpsOn ? "blue" : "gray"}>
          {httpsOn ? $_("On") : $_("Off")}
        </Tag>
        <span class="tile-foot">{$_("optional; needs the certificate")}</span>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h3>{$_("Last 24 hours")}</h3>
        <Button
          kind="ghost"
          size="small"
          icon={ArrowRight}
          on:click={() => gsNavigate("/stats")}
        >
          {$_("View stats")}
        </Button>
      </div>
      <div class="metrics">
        <div>
          <span class="tile-value">{fmt(summary?.total)}</span>
          <span class="tile-foot">{$_("requests")}</span>
        </div>
        <div>
          <span class="tile-value">{fmt(blocked24)}</span>
          <span class="tile-foot">{$_("blocked")} ({blockRate})</span>
        </div>
        <div>
          <span class="tile-value">{fmt(summary?.unique_clients)}</span>
          <span class="tile-foot">{$_("active clients")}</span>
        </div>
      </div>
      {#if summary?.top_blocked_domains?.length}
        <h4 class="subhead">{$_("Most blocked")}</h4>
        <StructuredList condensed flush>
          <StructuredListBody>
            {#each summary.top_blocked_domains.slice(0, 5) as item (item.key)}
              <StructuredListRow>
                <StructuredListCell>{item.key}</StructuredListCell>
                <StructuredListCell>{fmt(item.count)}</StructuredListCell>
              </StructuredListRow>
            {/each}
          </StructuredListBody>
        </StructuredList>
      {:else if summary}
        <p class="helper">{$_("No blocked requests in the last 24 hours.")}</p>
      {/if}
    </section>

    <div class="columns">
      <section class="panel">
        <div class="panel-head">
          <h3>{$_("Setup")}</h3>
          {#if setupDone}<Tag type="green">{$_("Complete")}</Tag>{/if}
        </div>
        {#if readiness.length > 0}
          <!-- A negative index keeps completed steps checked; Carbon's
               "current" marker would hide the last checkmark. -->
          <ProgressIndicator
            vertical
            preventChangeOnClick
            currentIndex={pendingIndex}
          >
            {#each readiness as step (step.name)}
              <ProgressStep
                complete={step.state === "ok"}
                invalid={step.state === "failed"}
                label={stepLabel(step.name)}
                secondaryLabel={stepDetail(step.name)}
              />
            {/each}
          </ProgressIndicator>
        {/if}

        <p class="helper">
          {$_(
            "The protection check asks the running DNS server to resolve a controlled test domain and records the decision it made.",
          )}
        </p>
        <Button
          size="small"
          on:click={runProtectionCheck}
          disabled={checking || !status.dns_running}
        >
          {checking ? $_("Checking...") : $_("Run protection check")}
        </Button>

        {#if checkState}
          <div class="check-result">
            <Tag type={checkState === "ok" ? "green" : "red"}>
              {checkState === "ok" ? $_("Protected") : $_("Not protected")}
            </Tag>
            <p>{checkDetail}</p>
            {#if check}
              <StructuredList condensed flush>
                <StructuredListBody>
                  <StructuredListRow>
                    <StructuredListCell noWrap>{$_("Test domain")}</StructuredListCell>
                    <StructuredListCell>{check.domain}</StructuredListCell>
                  </StructuredListRow>
                  <StructuredListRow>
                    <StructuredListCell noWrap>{$_("Answer")}</StructuredListCell>
                    <StructuredListCell>{check.rcode}</StructuredListCell>
                  </StructuredListRow>
                  {#if check.cname}
                    <StructuredListRow>
                      <StructuredListCell noWrap>{$_("Explanation")}</StructuredListCell>
                      <StructuredListCell>{check.cname}</StructuredListCell>
                    </StructuredListRow>
                  {/if}
                  <StructuredListRow>
                    <StructuredListCell noWrap>{$_("Upstream")}</StructuredListCell>
                    <StructuredListCell>{check.upstream}</StructuredListCell>
                  </StructuredListRow>
                </StructuredListBody>
              </StructuredList>
            {/if}
            {#if checkAction}
              <InlineNotification
                kind="error"
                lowContrast
                hideCloseButton
                title={$_("Protection check failed")}
                subtitle={checkAction}
              />
            {/if}
          </div>
        {/if}
      </section>

      <section class="panel">
        <h3>{$_("Connect a device")}</h3>
        <p class="helper">
          {$_(
            "Set the DNS server on a device, or in your router's DHCP settings, to the GateSentry address. Devices that use another resolver are not filtered.",
          )}
        </p>
        <StructuredList condensed flush>
          <StructuredListBody>
            <StructuredListRow>
              <StructuredListCell noWrap>{$_("DNS listener")}</StructuredListCell>
              <StructuredListCell>
                <code>{status.listen_addr}:{status.listen_port}</code>
              </StructuredListCell>
            </StructuredListRow>
            <StructuredListRow>
              <StructuredListCell noWrap>{$_("Upstream resolver")}</StructuredListCell>
              <StructuredListCell><code>{status.upstream}</code></StructuredListCell>
            </StructuredListRow>
          </StructuredListBody>
        </StructuredList>
        <p class="helper">{$_("Test from a device:")}</p>
        <CodeSnippet type="single" code={`dig @${dnsHost()} example.com`} />
        <div class="links">
          <Button kind="tertiary" size="small" on:click={() => gsNavigate("/rules")}>
            {$_("Policies")}
          </Button>
          <Button kind="ghost" size="small" on:click={() => gsNavigate("/devices")}>
            {$_("Devices")}
          </Button>
          <Button kind="ghost" size="small" on:click={() => gsNavigate("/dns")}>
            {$_("DNS settings")}
          </Button>
        </div>
      </section>
    </div>
  {/if}
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    max-width: 80rem;
  }
  .page-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    flex-wrap: wrap;
  }
  .lede {
    max-width: 44rem;
    margin-top: 0.5rem;
    color: var(--cds-text-secondary, #525252);
  }
  .helper {
    max-width: 44rem;
    margin: 0.75rem 0;
    color: var(--cds-text-helper, #6f6f6f);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
    gap: 1rem;
  }
  .tile,
  .panel {
    background: var(--cds-ui-01, #f4f4f4);
    padding: 1rem 1.25rem;
    min-width: 0;
  }
  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.4rem;
  }
  .tile-label {
    font-size: 0.875rem;
    color: var(--cds-text-secondary, #525252);
  }
  .tile-value {
    display: block;
    font-size: 2rem;
    font-weight: 300;
    line-height: 1.2;
  }
  .tile-foot {
    font-size: 0.75rem;
    color: var(--cds-text-helper, #6f6f6f);
  }
  .panel-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  .panel > h3 {
    margin-bottom: 0.5rem;
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
    gap: 1rem;
  }
  .subhead {
    margin: 1.25rem 0 0.5rem;
  }
  .columns {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr));
    gap: 1rem;
  }
  @media (max-width: 480px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
  .check-result {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.75rem;
    margin-top: 1rem;
  }
  .check-result > p {
    margin: 0;
  }
  .links {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    margin-top: 1rem;
  }
  code {
    font-family: var(--cds-code-01-font-family, monospace);
    word-break: break-all;
  }
</style>
