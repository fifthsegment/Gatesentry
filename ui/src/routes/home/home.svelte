<script lang="ts">
  import { Column, Row } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  import imageBin from "../../assets/front.jpg?inline";
  import { Laptop, Mobile } from "carbon-icons-svelte";
  import Modal from "../../components/modal.svelte";
  import Instructionsphone from "./instructionsphone.svelte";
  import Instructionscomputer from "./instructionscomputer.svelte";
  let loggedIn = false;
  $: loggedIn = $store.api.loggedIn;

  let modalOpenComputer = false;
  let modalOpenPhone = false;
  let server_url = "";
  $store.api.doCall("/status").then((json) => {
    if (json) {
      server_url = json.server_url;
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
      Gatesentry is running on <strong>{server_url}</strong>
    </div>
    <Row>
      <Column>
        <div
          class="text-center m-10-top clickable"
          on:click={() => (modalOpenComputer = true)}
        >
          <Laptop size={64} />
          <div>{$_("Setup Gatesentry on your computer.")}</div>
        </div>
        <Modal
          bind:open={modalOpenComputer}
          title={$_("1. Setup Gatesentry on your favorite browser.")}
          children={Instructionscomputer}
        />
      </Column>
      <Column>
        <div
          class="text-center m-10-top clickable"
          on:click={() => (modalOpenPhone = true)}
        >
          <Mobile size={64} />
          <div>{$_("Setup Gatesentry on your phone.")}</div>
        </div>
        <Modal
          bind:open={modalOpenPhone}
          title={$_("1. Setup Gatesentry on your favorite browser.")}
          children={Instructionsphone}
        />
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
