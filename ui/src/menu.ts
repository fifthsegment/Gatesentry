import {
  Filter,
  Home,
  LogoAnsibleCommunity,
  Catalog,
  GraphicalDataFlow,
  ServerDns,
  Settings,
  SwitchLayer_2,
  Network_4,
  UserAccess,
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
        text: "Block List",
        href: "/blockedurls",
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
        text: "Exception Hostnames",
        href: "/excludehosts",
        icon: Filter,
      },
      // {
      //   type: "link",
      //   text: "Excluded URLs",
      //   href: "/excludeurls",
      //   icon: Filter,
      // },
    ],
  },
  {
    type: "link",
    text: "Services",
    href: "/services",
    icon: SwitchLayer_2,
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
