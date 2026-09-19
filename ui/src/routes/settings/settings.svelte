<script lang="ts">
	import { onMount } from "svelte";
	import { _ } from "svelte-i18n";

  import ConnectedGeneralSettingInput from "../../components/connectedGeneralSettingInputs.svelte";
  import HttpsToggle from "../../components/httpsToggle.svelte";
  import InspectionStatus from "../../components/inspectionStatus.svelte";
  import ConnectedCertificateComposed from "../../components/connectedCertificateComposed.svelte";
  import ConnectedSettingInput from "../../components/connectedSettingInput.svelte";
	import { Breadcrumb, BreadcrumbItem, Button, InlineNotification, Tag, TextInput } from "carbon-components-svelte";
	import { store } from "../../store/apistore";

	let diagnostics = null;
	let bundlePreview = "";
	let loadingDiagnostics = false;
	let diagnosticsError = "";

	async function loadDiagnostics() {
		loadingDiagnostics = true;
		diagnosticsError = "";
		try {
			diagnostics = await $store.api.doCall("/diagnostics");
		} catch (error) {
			diagnosticsError = error instanceof Error ? error.message : "Diagnostics failed";
		} finally {
			loadingDiagnostics = false;
		}
	}

	async function previewBundle() {
		diagnosticsError = "";
		try {
			const bundle = await $store.api.doCall("/diagnostics/bundle");
			bundlePreview = JSON.stringify(bundle, null, 2);
		} catch (error) {
			diagnosticsError = error instanceof Error ? error.message : "Bundle preview failed";
		}
	}

	function downloadBundle() {
		if (!bundlePreview) return;
		const url = URL.createObjectURL(new Blob([bundlePreview + "\n"], { type: "application/json" }));
		const link = document.createElement("a");
		link.href = url;
		link.download = "gatesentry-support-bundle.json";
		link.click();
		URL.revokeObjectURL(url);
	}

	function tagType(status) { return status === "ok" ? "green" : status === "failed" ? "red" : "gray"; }
	onMount(loadDiagnostics);
</script>

<Breadcrumb style="margin-bottom: 10px;">
  <BreadcrumbItem href="/">Dashboard</BreadcrumbItem>
  <BreadcrumbItem>Settings</BreadcrumbItem>
</Breadcrumb>

<h2>Settings</h2>

<br />

<ConnectedGeneralSettingInput
  keyName="log_location"
  title={$_("Log Location")}
  labelText={$_("Log Location")}
  type="text"
  helperText=""
/>
<br />
<TextInput
  value={$store.api.username}
  type="text"
  title={$_("Admin username")}
  labelText={$_("Admin username")}
  disabled={true}
/>

<HttpsToggle />

<InspectionStatus />

<ConnectedCertificateComposed />

<section class="diagnostics gs-card">
	<h3>{$_("Gateway diagnostics")}</h3>
	<p>{$_("Check gateway health and create a support bundle that excludes credentials, private keys, image data, device identities, and browsing history.")}</p>
	{#if diagnosticsError}<InlineNotification kind="error" title="Diagnostics error" subtitle={diagnosticsError} on:close={() => diagnosticsError = ""} />{/if}
	<div class="actions">
		<Button size="small" disabled={loadingDiagnostics} on:click={loadDiagnostics}>{loadingDiagnostics ? $_("Checking…") : $_("Run diagnostics")}</Button>
		<Button size="small" kind="secondary" on:click={previewBundle}>{$_("Preview support bundle")}</Button>
		<Button size="small" kind="tertiary" disabled={!bundlePreview} on:click={downloadBundle}>{$_("Download preview")}</Button>
	</div>
	{#if diagnostics}
		<p class="overall"><strong>{$_("Overall status")}:</strong> <Tag type={tagType(diagnostics.overall)} size="sm">{diagnostics.overall}</Tag></p>
		<div class="checks">
			{#each diagnostics.checks || [] as check}
				<article>
					<header><strong>{check.name}</strong><Tag type={tagType(check.status)} size="sm">{check.status}</Tag></header>
					<p>{check.message}</p>
					{#if check.recovery}<p class="recovery"><strong>{$_("Recovery")}:</strong> {check.recovery}</p>{/if}
				</article>
			{/each}
		</div>
	{/if}
	{#if bundlePreview}
		<label for="bundle-preview">{$_("Redacted bundle preview")}</label>
		<textarea id="bundle-preview" readonly value={bundlePreview} rows="16"></textarea>
	{/if}
</section>

<style>
	.diagnostics { margin-top: 2rem; padding: 1.5rem; }
	.diagnostics > p { max-width: 48rem; margin: .5rem 0 1rem; color: var(--cds-text-secondary, #525252); }
	.actions { display: flex; flex-wrap: wrap; gap: .5rem; margin-bottom: 1.25rem; }
	.overall { display: flex; align-items: center; gap: .5rem; }
	.checks { display: grid; gap: .75rem; margin: 1rem 0; }
	.checks article { border: 1px solid var(--cds-border-subtle, #e0e0e0); padding: 1rem; }
	.checks header { display: flex; align-items: center; justify-content: space-between; text-transform: capitalize; }
	.checks p { margin: .5rem 0 0; }
	.recovery { color: var(--cds-text-secondary, #525252); }
	label { display: block; font-weight: 600; margin: 1rem 0 .5rem; }
	textarea { width: 100%; padding: .75rem; font-family: monospace; resize: vertical; }
</style>
