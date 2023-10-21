<script lang="ts">
  import { _ } from "svelte-i18n";
  import { store } from "../store/apistore";
  import { Download } from "carbon-icons-svelte";

  let link = `/api/files/certificate`;
</script>

<a href={link} target="_blank"
  ><Download style="position:relative;top:3px" />{$_("Download Certificate")}</a
>
