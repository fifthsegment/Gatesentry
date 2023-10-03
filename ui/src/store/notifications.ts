import type { ToastNotificationProps } from "carbon-components-svelte/types/Notification/ToastNotification.svelte";
import { writable } from "svelte/store";

const { subscribe, update } = writable([] as Notification[]);

type Notification = {
  id: number;
  title: string;
  subtitle: string;
  kind: ToastNotificationProps["kind"];
  timeout?: number;
};

export const notificationstore = {
  subscribe,
  add: ({ title, subtitle, kind, timeout = 1000 }) =>
    update((currentArray) => {
      return [
        ...currentArray,
        {
          id: currentArray.length + 1,
          title: title,
          subtitle: subtitle,
          kind: kind,
          timeout: timeout,
        },
      ];
    }),
  remove: (notification: Notification) =>
    update((currentArray) => {
      return currentArray.filter((n) => n.id !== notification.id);
    }),
  refresh: () => update((s) => s),
};
