<script lang="ts">
  import ConnectedCertificateBasic from "./connectedCertificateBasic.svelte";
  import { _ } from "svelte-i18n";

  import { store } from "../store/apistore";
  import { onMount } from "svelte";
  import DownloadCertificateLink from "./downloadCertificateLink.svelte";
  import { Edit, EditOff, TagEdit } from "carbon-icons-svelte";

  let info = null;

  let editMode = false;

  onMount(async () => {
    info = await $store.api.doCall("/certificate/info");
  });
</script>

<br />
<label class="bx--label">{$_("MITM Filtering Certificate")}</label>
<br />
<div on:click={() => (editMode = !editMode)} style="margin-bottom: 10px;">
  <TagEdit style="position:relative; top:3px;" />

  <label style="font-size:0.8em; cursor:pointer"
    >{editMode ? $_("View") : $_("Edit")}</label
  >
</div>

{#if info !== null}
  <div class="simple-border" style="margin-top: 3px;">
    <div>
      {$_("Certificate Issuer Name: ")}
      {info.name}
    </div>
    <br />
    <div>
      {$_("Certificate Expiry: ")}
      {info.expiry}
    </div>
  </div>
  <br />
{/if}

{#if editMode}
  <ConnectedCertificateBasic
    settingName="capem"
    label={$_("HTTPS Filtering - Certificate")}
  />

  <ConnectedCertificateBasic
    settingName="keypem"
    label={$_("HTTPS Filtering - Key")}
  />
  <br />
  <DownloadCertificateLink />
{/if}
