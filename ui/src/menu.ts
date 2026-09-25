import type { ComponentType } from "svelte";
import {
  Filter,
  Locked,
  Home,
  Catalog,
  GraphicalDataFlow,
  ServerDns,
  Settings,
  SwitchLayer_2,
  Network_4,
  UserAccess,
  Rule,
  Devices,
} from "carbon-icons-svelte";
import { getBasePath } from "./lib/navigate";

type MenuIcon = ComponentType<any>;

export type MenuLink = {
  type: "link";
  text: string;
  href: string;
  icon: MenuIcon;
};

export type MenuGroup = {
  type: "menu";
  text: string;
  icon: MenuIcon;
  children: MenuLink[];
};

export type MenuItem = MenuLink | MenuGroup;

export const menuItems: MenuItem[] = [
  { type: "link", text: "Home", href: "/", icon: Home },
  { type: "link", text: "Logs", href: "/logs", icon: Catalog },
  { type: "link", text: "Settings", href: "/settings", icon: Settings },
  { type: "link", text: "DNS", href: "/dns", icon: ServerDns },
  { type: "link", text: "Devices", href: "/devices", icon: Devices },
  { type: "link", text: "Policies", href: "/rules", icon: Rule },
  { type: "link", text: "Services", href: "/services", icon: SwitchLayer_2 },
  {
    type: "menu",
    text: "HTTPS inspection",
    icon: Locked,
    children: [
      { type: "link", text: "Blocked keywords", href: "/blockedkeywords", icon: Filter },
      { type: "link", text: "Blocked content types", href: "/blockedfiletypes", icon: Filter },
      { type: "link", text: "Sites not inspected", href: "/excludehosts", icon: Filter },
      { type: "link", text: "URLs not inspected", href: "/excludeurls", icon: Filter },
    ],
  },
  { type: "link", text: "Stats", href: "/stats", icon: GraphicalDataFlow },
  { type: "link", text: "AI", href: "/ai", icon: Network_4 },
  { type: "link", text: "Users", href: "/users", icon: UserAccess },
];

export function routeHref(href: string): string {
  return getBasePath() + href;
}

export function currentRoute(pathname = window.location.pathname): string {
  const base = getBasePath();
  if (base && (pathname === base || pathname.startsWith(base + "/"))) {
    pathname = pathname.slice(base.length) || "/";
  }
  return pathname || "/";
}

export function isRouteActive(href: string, pathname = currentRoute()): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(href + "/");
}

export function isMenuActive(item: MenuItem, pathname = currentRoute()): boolean {
  return item.type === "link"
    ? isRouteActive(item.href, pathname)
    : item.children.some((child) => isRouteActive(child.href, pathname));
}
