import { expect, test } from "vitest";
import settingsSource from "../../routes/settings/settings.svelte?raw";
import deviceListSource from "../../routes/devices/devicelist.svelte?raw";

const settings = settingsSource.replace(/\s+/g, " ");
const devices = deviceListSource.replace(/\s+/g, " ");

test("settings has a dedicated authenticated Tailscale status and config card", () => {
  expect(settings).toContain('$store.api.doCall("/tailscale/status")');
  expect(settings).toContain(
    '$store.api.doCall("/tailscale/config", "put", { enabled })',
  );
  expect(settings).toContain("tailscaleStatus.detected");
  expect(settings).toContain("tailscaleStatus.backend_state");
  expect(settings).toContain("tailscaleStatus.self_name");
  expect(settings).toContain("tailscaleStatus.last_refresh");
  expect(settings).toContain("tailscaleStatus.message");
  expect(settings).toContain('status === "connected"');
  expect(settings).toContain('status === "unavailable"');
  expect(settings).toContain('status === "degraded"');
  expect(settings).toContain(
    "kind={tailscaleStatusKind(tailscaleStatus.state)}",
  );
  expect(settings).toContain("subtitle={tailscaleStatus.message}");
});

test("settings explains explicit linking and Tailscale MAC limitations", () => {
  expect(settings).toContain("Peer suggestions are hints only");
  expect(settings).toContain("only after you choose it and confirm the link");
  expect(settings).toContain("normally does not supply a hardware MAC address");
  expect(settings).toContain("Wake-on-LAN MAC addresses");
  expect(settings).toContain("informational");
});

test("device inventory exposes linked Tailscale peers and opens link management", () => {
  expect(devices).toContain("tailscale_nodes");
  expect(devices).toContain("tailscale_display");
  expect(devices).toContain("Manage Tailscale links");
  expect(devices).toContain("on:tailscaleChanged={loadDevices}");
  expect(devices).toContain("setInterval(loadDevices, 30000)");
});

test("device inventory uses explicit Carbon states and confirmation", () => {
  expect(devices).toContain("ResourceState");
  expect(devices).toContain("No devices discovered yet");
  expect(devices).toContain('size="compact"');
  expect(devices).toContain("<ConfirmDialog");
  expect(devices).toContain("on:submit={confirmRemoval}");
  expect(devices).not.toContain("confirm(");
  expect(devices).toContain('class="inventory-table"');
  expect(devices).toContain("overflow-x: auto");
});
