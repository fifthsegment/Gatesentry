import { init, register } from "svelte-i18n";

export const setupI18n = () => {
  register("en", () => import("./en.json"));
  init({
    fallbackLocale: "en",
    initialLocale: "en",
  });
};
