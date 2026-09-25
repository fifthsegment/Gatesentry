import "carbon-components-svelte/css/g10.css";
import "./app.css";
import App from "./App.svelte";

const target = document.getElementById("app");
if (!target) throw new Error("GateSentry app root was not found");

const app = new App({ target });

export default app;
