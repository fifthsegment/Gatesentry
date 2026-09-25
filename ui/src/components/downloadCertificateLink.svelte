<script lang="ts">
  import { Button } from "carbon-components-svelte";
  import { Download } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { getBasePath } from "../lib/navigate";

  export let kind: "primary" | "secondary" | "tertiary" | "ghost" = "tertiary";
  export let size: "default" | "field" | "small" | "lg" | "xl" = "small";

  const link = getBasePath() + "/api/files/certificate";
</script>

<Button href={link} icon={Download} {kind} {size} download>
  {$_("Download certificate")}
</Button>
