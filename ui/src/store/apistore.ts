// commonStore.js or commonStore.ts
import { writable } from "svelte/store";
import AppAPI from "../lib/api";
import { navigate } from "svelte-routing";

// Create a writable store with the class instance
const api = new AppAPI();

const { subscribe, update } = writable({ api: api });

export const store = {
  subscribe,
  loginSuccesful: (jwtToken: string) =>
    update((s) => {
      s.api.setLoggedIn(jwtToken);
      return s;
    }),
  logout: () =>
    update((s) => {
      s.api.setLoggedOut();
      return s;
    }),
  refresh: () => update((s) => s),
};

api.onUnauthorized(() => {
  store.logout();
});
