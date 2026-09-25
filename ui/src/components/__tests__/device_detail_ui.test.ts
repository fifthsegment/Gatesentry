import { expect, test } from "vitest";
import deviceDetailSource from "../../routes/devices/devicedetail.svelte?raw";

// Prettier wraps long text nodes and attributes across lines; normalize
// whitespace so assertions on user-facing strings stay stable.
const source = deviceDetailSource.replace(/\s+/g, " ");

test("device detail assigns a policy explicitly", () => {
  expect(source).toContain('"/api/policy"');
  expect(source).toContain('POLICY_BASE + "/groups"');
  expect(source).toContain("/assignment");
  expect(source).toContain("Default policy");
  expect(source).toContain("group_id: selectedGroup");
  expect(source).toContain(
    "A device without a policy of its own uses the default policy.",
  );
});

test("device detail shows effective protection without overclaiming", () => {
  expect(source).toContain("/policy");
  expect(source).toContain("confidence");
  expect(source).toContain("coverage?.summary");
  expect(source).toContain("effective_policy");
  expect(source).toContain("coverage?.caveats");
  expect(source).toContain("so the default policy applies");
  expect(source).toContain("shared address");
  expect(source).toContain("stale observation");
});

test("device detail labels metadata as non-enforcing", () => {
  expect(source).toContain(
    "Descriptive label only — it does not change filtering.",
  );
  expect(source).toContain("metadata_note");
});

test("device detail links decision history to the log view", () => {
  expect(source).toContain("/activity");
  expect(source).toContain("View full decision history");
  expect(source).toContain('"/logs"');
  expect(source).toContain("decision history");
});

test("device detail saves labels through the modal submit event", () => {
  // Carbon's ModalFooter primary button calls the ComposedModal submit
  // context, so the handler must be on the modal, not the footer.
  expect(source).toContain("on:submit={save}");
  expect(source).not.toContain("on:click:button--primary");
});

test("device activity refreshes only while the modal is open", () => {
  expect(source).toContain("activity?since=86400&limit=100");
  expect(source).toContain("setTimeout(loadActivity, 5000)");
  expect(source).toContain("activityController?.abort()");
  expect(source).toContain("clearTimeout(activityTimer)");
  expect(source).toContain("!open ||");
  expect(source).toContain(
    "$: if (!open && activityDeviceID) stopActivity(true)",
  );
  expect(source).toContain("requestedDeviceID === activityDeviceID");
  expect(source).toContain("requestGeneration === activityGeneration");
});

test("device detail uses four focused Carbon tabs", () => {
  expect(source).toContain('<Tabs type="container" autoWidth');
  expect(source).toContain('<Tab label="Overview" />');
  expect(source).toContain('<Tab label="Policy" />');
  expect(source).toContain('<Tab label="Connections" />');
  expect(source).toContain('<Tab label="Identity & activity" />');
  expect(source.match(/<TabContent>/g)).toHaveLength(4);
});

test("device detail links and unlinks Tailscale peers only after manual action", () => {
  expect(source).toContain('TAILSCALE_BASE + "/peers"');
  expect(source).toContain("encodeURIComponent(peer.node_id)");
  expect(source).toContain(
    "body: JSON.stringify({ node_id: selectedPeerNodeID })",
  );
  expect(source).toContain("bind:selected={selectedPeerNodeID}");
  expect(source).toContain("disabled={tailscaleSaving || !selectedPeerNodeID}");
  expect(source).toContain("Suggestions are shown as hints only");
  expect(source).toContain("never select or link a peer automatically");
  expect(source).toContain("pendingUnlink = peer");
  expect(source).toContain("on:submit={confirmUnlink}");
  expect(source).not.toContain("confirm(");
  expect(source).not.toContain("selectedPeerNodeID = peer.suggestion");
});

test("device detail explains Tailscale MAC limitations", () => {
  expect(source).toContain("normally does not provide a hardware MAC address");
  expect(source).toContain("Wake-on-LAN");
  expect(source).toContain("informational");
  expect(source).toContain("peer.wol_macs");
});

test("device detail treats manager state separately from request errors", () => {
  expect(source).toContain("tailscaleState = data");
  expect(source).toContain(
    'data.state === "connected" ? data.peers || [] : []',
  );
  expect(source).toContain('tailscaleState?.state === "connected"');
  expect(source).toContain('if (state === "connected") return "success"');
  expect(source).toContain(
    'if (state === "degraded" || state === "connecting") return "warning"',
  );
  expect(source).toContain(
    'if (state === "unavailable" || state === "stopped") return "error"',
  );
  expect(source).toContain(
    "subtitle={tailscaleState.message || tailscaleState.backend_state}",
  );
});
