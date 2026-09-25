<script lang="ts">
  import { Button } from "carbon-components-svelte";
  import { Download } from "carbon-icons-svelte";
  import { _ } from "svelte-i18n";
  import { getBasePath } from "../lib/navigate";

  const link = getBasePath() + "/api/files/certificate";
</script>

<Button href={link} icon={Download} kind="tertiary" size="small" download>
  {$_("Download certificate")}
</Button>
