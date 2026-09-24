import {
  Filter,
  Locked,
  Home,
  LogoAnsibleCommunity,
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

let menuItems = [
  {
    type: "link",
    text: "Home",
    href: "/",
    icon: Home,
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
    text: "DNS",
    href: "/dns",
    icon: ServerDns,
  },
  {
    type: "link",
    text: "Devices",
    href: "/devices",
    icon: Devices,
  },
  {
    type: "link",
    text: "Policies",
    href: "/rules",
    icon: Rule,
  },
  {
    type: "link",
    text: "Services",
    href: "/services",
    icon: SwitchLayer_2,
  },
  {
    type: "menu",
    text: "HTTPS inspection",
    icon: Locked,
    children: [
      {
        type: "link",
        text: "Blocked keywords",
        href: "/blockedkeywords",
        icon: Filter,
      },
      {
        type: "link",
        text: "Blocked content types",
        href: "/blockedfiletypes",
        icon: Filter,
      },
      {
        type: "link",
        text: "Sites not inspected",
        href: "/excludehosts",
        icon: Filter,
      },
    ],
  },
  {
    type: "link",
    text: "Stats",
    href: "/stats",
    icon: GraphicalDataFlow,
  },
  {
    type: "link",
    text: "AI",
    href: "/ai",
    icon: Network_4,
  },
  {
    type: "link",
    text: "Users",
    href: "/users",
    icon: UserAccess,
  }
];

export { menuItems };
