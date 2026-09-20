<script lang="ts">
  import { onMount } from "svelte";
  import {
    Button,
    CodeSnippet,
    Column,
    InlineNotification,
    ProgressIndicator,
    ProgressStep,
    Row,
    StructuredList,
    StructuredListBody,
    StructuredListCell,
    StructuredListRow,
    Tag,
  } from "carbon-components-svelte";
  import { Restart } from "carbon-icons-svelte";
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
  type Notice = { key: string; title: string; subtitle: string };

  // The backend reports six steps. Only these four carry a state that setup can
  // actually reach: "clients" is an instruction and "https_inspection" is
  // optional, so both are described outside the readiness indicator.
  const measuredSteps = [
    "resolver",
    "blocklist",
    "first_device",
    "protection_check",
  ];

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
      return `${$_("First explained protection recorded")}: ${new Date(
        status.progress.first_explained_protection_at,
      ).toLocaleString()}`;
    }
    return $_("Not verified yet");
  };

  const collectNotices = (current: OnboardingStatus): Notice[] => {
    const notices: Notice[] = [];
    // The readiness steps already show each check's detail, so the resolver and
    // blocklist notices carry only the fix. The listener check has no step of
    // its own, so it keeps both.
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

  const elapsedMinutes = () => {
    if (!status?.progress?.started_at) return null;
    const started = new Date(status.progress.started_at).getTime();
    const protectedAt = status.progress.first_explained_protection_at
      ? new Date(status.progress.first_explained_protection_at).getTime()
      : Date.now();
    if (Number.isNaN(started) || Number.isNaN(protectedAt)) return null;
    return Math.max(0, Math.round((protectedAt - started) / 60000));
  };

  // A loopback listener address is not reachable from other devices, so the
  // snippet keeps the placeholder until GateSentry is bound to a real address.
  const unreachableHosts = ["", "0.0.0.0", "::", "[::]", "127.0.0.1", "::1"];
  const dnsHost = () => {
    const addr = status?.listen_addr || "";
    return unreachableHosts.includes(addr) ? "<gatesentry-host-ip>" : addr;
  };

  $: readiness = (status?.steps || []).filter((step) =>
    measuredSteps.includes(step.name),
  );
  $: pendingIndex = readiness.findIndex((step) => step.state !== "ok");
  $: notices = status ? collectNotices(status) : [];
  $: protectedYet = Boolean(status?.progress.first_explained_protection_at);

  onMount(loadStatus);
</script>

<Row>
  <Column>
    <h2>{$_("Gateway status")}</h2>
    <p class="lede">
      {$_(
        "GateSentry is a self-hosted internet safety gateway for a home or small office. It answers DNS for the devices that use it, blocks the domains on your blocklists, and records the decision behind each block.",
      )}
    </p>

    {#if statusError}
      <InlineNotification
        kind="error"
        title={$_("Setup status unavailable")}
        subtitle={statusError}
        on:close={() => (statusError = "")}
      />
      <Button kind="secondary" size="small" icon={Restart} on:click={loadStatus}>
        {$_("Retry status check")}
      </Button>
    {/if}

    {#if status}
      <section>
        <div class="section-head">
          <h3>{$_("Readiness")}</h3>
          <Tag type={protectedYet ? "green" : "red"}>
            {protectedYet ? $_("Protected") : $_("Not protected yet")}
          </Tag>
        </div>

        {#if readiness.length > 0}
          <!-- A negative index keeps every step in its final state once setup
               is complete: Carbon draws the "current" marker with a pending
               icon, which would hide the checkmark on the last step. -->
          <ProgressIndicator vertical preventChangeOnClick currentIndex={pendingIndex}>
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

        {#each notices as notice (notice.key)}
          <InlineNotification
            kind="error"
            lowContrast
            hideCloseButton
            title={notice.title}
            subtitle={notice.subtitle}
          />
        {/each}

        <p class="helper">
          {$_(
            "DNS blocking acts on domains. It cannot inspect or explain URL, MIME, keyword, or image-content decisions.",
          )}
        </p>
        <Button kind="secondary" size="small" icon={Restart} on:click={loadStatus}>
          {$_("Refresh status")}
        </Button>
      </section>

      <section>
        <h3>{$_("Verify protection")}</h3>
        <p class="lede">
          {$_(
            "Startup is not protection. This asks the running GateSentry DNS server to resolve a controlled test domain and records the decision it made.",
          )}
        </p>
        <Button
          on:click={runProtectionCheck}
          disabled={checking || !status.dns_running}
        >
          {checking ? $_("Checking...") : $_("Run protection check")}
        </Button>

        {#if checkState}
          <div class="check-result">
            <Tag type={checkState === "ok" ? "green" : "red"}>
              {checkState === "ok" ? $_("Protected") : $_("Not protected yet")}
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

        {#if status.progress.first_explained_protection_at && elapsedMinutes() !== null}
          <p class="helper">
            {$_("Time from setup start to first explained protection")}:
            {elapsedMinutes()}
            {$_("minutes")} ({$_("target")} ≤ 10).
          </p>
        {/if}
      </section>

      <section>
        <h3>{$_("Point devices at GateSentry")}</h3>
        <p class="lede">
          {$_(
            "Set the DNS server on a device, or the DHCP/DNS setting on your router, to the GateSentry host address.",
          )}
        </p>
        <CodeSnippet type="single" code={`dig @${dnsHost()} example.com`} />
        <p class="helper">
          {$_("DNS listener")}:
          <code class="listen-address">{status.listen_addr}:{status.listen_port}</code
          >. {$_(
            "Devices that use GateSentry for DNS are the only devices covered. Devices that use a different resolver are unverified.",
          )}
        </p>
        <Button kind="ghost" size="small" on:click={() => gsNavigate("/dns")}>
          {$_("DNS settings")}
        </Button>
      </section>

      <section>
        <h3>{$_("HTTPS inspection")}</h3>
        <p class="helper">
          {$_(
            "Optional and advanced: it enables URL, MIME, keyword, and content filtering for devices that trust the GateSentry CA certificate. It is not required for the steps above. Devices that have not installed the certificate, and traffic that bypasses the proxy, remain outside HTTPS inspection.",
          )}
        </p>
        <Button kind="ghost" size="small" on:click={() => gsNavigate("/settings")}>
          {$_("Settings")}
        </Button>
      </section>
    {/if}
  </Column>
</Row>

<style>
  .lede {
    max-width: 42rem;
    color: var(--cds-text-secondary, #525252);
  }
  .helper {
    max-width: 48rem;
    color: var(--cds-text-helper, #6f6f6f);
  }
  section {
    margin-top: 2rem;
  }
  .section-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .listen-address {
    font-family: var(--cds-code-01-font-family, monospace);
  }
  .check-result {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.75rem;
    margin-top: 1rem;
  }
  .check-result > p {
    max-width: 48rem;
    margin: 0;
  }
</style>
