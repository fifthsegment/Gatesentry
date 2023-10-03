import {
  Filter,
  Home,
  LogoAnsibleCommunity,
  Catalog,
  GraphicalDataFlow,
  ServerDns,
  Settings,
  SwitchLayer_2,
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
    type: "menu",
    text: "Filters",
    icon: Filter,
    children: [
      {
        type: "link",
        text: "Keywords to Block",
        href: "/blockedkeywords",
        icon: Filter,
      },
      {
        type: "link",
        text: "Blocked URLs",
        href: "/blockedurls",
        icon: Filter,
      },
      {
        type: "link",
        text: "Blocked file types",
        href: "/blockedfiletypes",
        icon: Filter,
      },
      {
        type: "link",
        text: "Excluded Hosts",
        href: "/excludehosts",
        icon: Filter,
      },
      {
        type: "link",
        text: "Excluded URLs",
        href: "/excludeurls",
        icon: Filter,
      },
    ],
  },
  {
    type: "link",
    text: "Switches",
    href: "/switches",
    icon: SwitchLayer_2,
  },
  {
    type: "link",
    text: "Stats",
    href: "/stats",
    icon: GraphicalDataFlow,
  }
];

export { menuItems };
