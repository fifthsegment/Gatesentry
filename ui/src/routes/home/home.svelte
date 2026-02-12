<script lang="ts">
  import { Column, Row } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  import imageBin from "../../assets/front.jpg?inline";
  import LaptopIcon from "../../components/icons/LaptopIcon.svelte";
  import MobileIcon from "../../components/icons/MobileIcon.svelte";
  import Modal from "../../components/modal.svelte";
  import Instructionsphone from "./instructionsphone.svelte";
  import Instructionscomputer from "./instructionscomputer.svelte";
  let loggedIn = false;
  $: loggedIn = $store.api.loggedIn;

  let modalOpenComputer = false;
  let modalOpenPhone = false;
  let dns_address = "";
  let dns_port = "";
  let proxy_port = "";
  $store.api.doCall("/status").then((json) => {
    if (json) {
      dns_address = json.dns_address || "";
      dns_port = json.dns_port || "53";
      proxy_port = json.proxy_port || "10413";
    }
  });
</script>

<h2>{$_("Welcome to Gatesentry")}</h2>
<br />
<div>
  {$_("Click on the menu to the left to get started.")}
</div>

<Row
  ><Column>
    <div class="simple-border">
      {#if dns_address}
        Host: <strong>{dns_address}</strong> &mdash; DNS port
        <strong>{dns_port}</strong>, Proxy port <strong>{proxy_port}</strong>
      {:else}
        Gatesentry is starting…
      {/if}
    </div>
    <Row>
      <Column>
        <div
          class="text-center m-10-top clickable"
          role="button"
          tabindex="0"
          on:click={() => (modalOpenComputer = true)}
          on:keypress={() => (modalOpenComputer = true)}
        >
          <LaptopIcon size={128} />
          <div>{$_("Setup Gatesentry on your computer.")}</div>
        </div>
        <Modal
          bind:open={modalOpenComputer}
          title={$_("1. Setup Gatesentry on your favorite browser.")}
        >
          <Instructionscomputer />
        </Modal>
      </Column>
      <Column>
        <div
          class="text-center m-10-top clickable"
          role="button"
          tabindex="0"
          on:click={() => (modalOpenPhone = true)}
          on:keypress={() => (modalOpenPhone = true)}
        >
          <MobileIcon size={128} />
          <div>{$_("Setup Gatesentry on your phone.")}</div>
        </div>
        <Modal
          bind:open={modalOpenPhone}
          title={$_("1. Setup Gatesentry on your favorite browser.")}
        >
          <Instructionsphone />
        </Modal>
      </Column>
    </Row>
    <Row>
      <Column>
        <br />
        <br />
        <h4>{$_("What is Gatesentry?")}</h4>
        <div style="line-height: 1.7">
          {$_(
            "Gatesentry is a DNS+Proxy server combo that filters out malicious content and policy violations.",
          )}
        </div>
        <br />
        <br />

        <h4>{$_("Why do we need MITM filtering?")}</h4>
        <div style="line-height: 1.7">
          Today, Man-in-the-Middle (MITM) filtering is crucial for inspecting
          encrypted traffic, such as HTTPS, which comprises 90% of all websites.
          It intercepts, decrypts, and re-encrypts traffic to filter out
          malicious content or policy violations, addressing the challenge posed
          by the rise in HTTPS traffic, which traditional network filters cannot
          inspect. Through MITM filtering, organizations can effectively
          mitigate cyber threats and ensure compliance with network policies
          amidst the prevalent use of encryption for online communications.
        </div>
      </Column>
    </Row>
  </Column><Column
    ><div style="margin: 0 auto" class="text-center">
      <img src={imageBin} style="width:500px; border:1px solid gray;" />
      <pre>Once properly setup, Gatesentry can filter traffic on all your devices.</pre>
    </div></Column
  ></Row
>
