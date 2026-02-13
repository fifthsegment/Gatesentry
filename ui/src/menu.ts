import {
  Filter,
  Home,
  LogoAnsibleCommunity,
  Catalog,
  GraphicalDataFlow,
  ServerDns,
  Settings,
  Network_4,
  UserAccess,
  Rule,
  Devices,
  ListBoxes,
} from "carbon-icons-svelte";

let menuItems = [
  {
    type: "link",
    text: "Home",
    href: "/",
    icon: Home,
  },
  {
    type: "link",
    text: "Domain Lists",
    href: "/domainlists",
    icon: ListBoxes,
  },
  {
    type: "link",
    text: "DNS",
    href: "/dns",
    icon: ServerDns,
  },
  {
    type: "link",
    text: "Proxy Rules",
    href: "/rules",
    icon: Rule,
  },
  {
    type: "link",
    text: "Devices",
    href: "/devices",
    icon: Devices,
  },
  {
    type: "link",
    text: "Users",
    href: "/users",
    icon: UserAccess,
  },
  {
    type: "link",
    text: "Keywords",
    href: "/blockedkeywords",
    icon: Filter,
  },
  {
    type: "link",
    text: "Stats",
    href: "/stats",
    icon: GraphicalDataFlow,
  },
  {
    type: "link",
    text: "Logs",
    href: "/logs",
    icon: Catalog,
  },
  {
    type: "link",
    text: "Settings",
    href: "/settings",
    icon: Settings,
  },
  {
    type: "link",
    text: "AI",
    href: "/ai",
    icon: Network_4,
  },
];

export { menuItems };
