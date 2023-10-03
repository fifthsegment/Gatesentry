import { init, register } from "svelte-i18n";

export const setupI18n = () => {
  console.log("[App] Setting up i18n");
  register("en", () => import("./en.json"));
  init({
    fallbackLocale: "en",
    initialLocale: "en",
  });
};
