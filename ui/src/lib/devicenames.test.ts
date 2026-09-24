import { expect, test } from "vitest";
import { buildDeviceNameMap, formatDeviceAddress } from "./devicenames";

test("maps current IPv4 and IPv6 addresses to trimmed manual names", () => {
  const names = buildDeviceNameMap([
    {
      manual_name: "  Kitchen tablet  ",
      ipv4: "192.0.2.10",
      ipv6: "2001:db8::10",
    },
  ]);
  expect(formatDeviceAddress(names, "192.0.2.10")).toBe(
    "Kitchen tablet (192.0.2.10)",
  );
  expect(formatDeviceAddress(names, "2001:db8::10")).toBe(
    "Kitchen tablet (2001:db8::10)",
  );
});

test("ignores inferred labels and falls back to the raw address", () => {
  const names = buildDeviceNameMap([
    {
      manual_name: "   ",
      ipv4: "192.0.2.20",
      display_name: "aa:bb:cc:dd:ee:ff",
      dns_name: "laptop",
    } as any,
  ]);
  expect(formatDeviceAddress(names, "192.0.2.20")).toBe("192.0.2.20");
  expect(formatDeviceAddress(names, "192.0.2.99")).toBe("192.0.2.99");
});

test("does not guess when two devices claim the same address", () => {
  const names = buildDeviceNameMap([
    { manual_name: "First", ipv4: "192.0.2.30" },
    { manual_name: "Second", ipv4: "192.0.2.30" },
  ]);
  expect(formatDeviceAddress(names, "192.0.2.30")).toBe("192.0.2.30");
});

test("treats an address shared with an unnamed device as ambiguous", () => {
  const names = buildDeviceNameMap([
    { manual_name: "Named", ipv4: "192.0.2.40" },
    { ipv4: "192.0.2.40" },
  ]);
  expect(formatDeviceAddress(names, "192.0.2.40")).toBe("192.0.2.40");
});

test("does not treat duplicate address fields on one device as ambiguity", () => {
  const names = buildDeviceNameMap([
    {
      manual_name: "Gateway",
      ipv4: "192.0.2.50",
      ipv6: "192.0.2.50",
    },
  ]);
  expect(formatDeviceAddress(names, "192.0.2.50")).toBe("Gateway (192.0.2.50)");
});
