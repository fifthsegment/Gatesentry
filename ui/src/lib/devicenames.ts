export type DeviceNameSource = {
  manual_name?: string;
  ipv4?: string;
  ipv6?: string;
};

// Device names are joined to decision rows using current addresses only. The
// raw address stays visible because an old row can outlive a DHCP assignment.
export function buildDeviceNameMap(
  devices: DeviceNameSource[] | undefined,
): Map<string, string> {
  const names = new Map<string, string>();
  const claims = new Map<string, number>();

  for (const device of devices || []) {
    for (const ip of new Set([device.ipv4, device.ipv6].filter(Boolean))) {
      claims.set(ip as string, (claims.get(ip as string) || 0) + 1);
    }
  }

  for (const device of devices || []) {
    const name = device.manual_name?.trim();
    if (!name) continue;

    for (const ip of new Set([device.ipv4, device.ipv6].filter(Boolean))) {
      if (claims.get(ip as string) === 1) names.set(ip as string, name);
    }
  }

  return names;
}

export function formatDeviceAddress(
  names: Map<string, string>,
  ip: string,
): string {
  const name = names.get(ip);
  return name ? `${name} (${ip})` : ip;
}
