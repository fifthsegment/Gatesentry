<script lang="ts">
  import { Button, FormLabel, TextInput } from "carbon-components-svelte";
  import { store } from "../../store/apistore";
  import { _ } from "svelte-i18n";
  import Certificate from "../../components/certificate.svelte";
  let data = null;
  let enable_https_filtering = null;
  const loadAPIData = () => {
    const url = "/settings/general_settings";

    $store.api.doCall(url).then(function (json) {
      data = JSON.parse(json.Value);
    });

    $store.api.doCall("/settings/enable_https_filtering").then((json) => {
      enable_https_filtering = json.Value;
    });
  };

  const toggleHttpsFilteringStatus = () => {
    const url = "/settings/enable_https_filtering";
    var datatosend = {
      key: "enable_https_filtering",
      value: enable_https_filtering == "true" ? "false" : "true",
    };
    $store.api.doCall(url, "post", datatosend).then(function (json) {
      console.log("json", json);
      loadAPIData();
      // that.mapDataToState(json);
    });
  };

  loadAPIData();
</script>

<h2>Settings</h2>

<TextInput
  title={$_("Log Location")}
  labelText={$_("Log Location")}
  value={data?.log_location}
/>

<TextInput
  helperText={$_("Leave blank to keep the current password")}
  type="password"
  title={$_("Password")}
  labelText={$_("Password")}
  value={data?.admin_password}
/>

<Certificate />

<div style="margin-top: 15px;">
  <FormLabel>{$_("HTTPS Filtering - Man In The Middle Filtering")}</FormLabel>
  <div>
    {enable_https_filtering == "true" ? "Enabled" : "Disabled"}
  </div>
  <div style="margin-top: 5px;">
    <Button size="small" on:click={toggleHttpsFilteringStatus}
      >{enable_https_filtering == "true" ? "Turn off" : "Turn on"}</Button
    >
  </div>
  <div>
    <div class="mitm-privacy-notice">
      <h3 class="notice-title">
        {$_("Privacy Notice for Enabling MITM Filtering")}
      </h3>

      <h4 class="section">Attention System Administrators:</h4>
      <p>
        Before enabling the Man-In-The-Middle (MITM) filtering on this proxy
        server, it is crucial to understand the significant privacy implications
        this feature holds. Proceed with utmost caution and awareness of the
        responsibilities involved.
      </p>

      <h4 class="section">Privacy Implications:</h4>
      <ol>
        <li>
          <b>Inspection of Encrypted Traffic:</b>
          <p>
            Enabling MITM filtering allows the inspection and potential
            modification of encrypted traffic passing through the proxy server.
            This action inherently accesses the content of communications,
            possibly revealing sensitive and personal data of users on the
            network.
          </p>
        </li>
        <li>
          <b>Potential Infringement on Privacy Rights:</b>
          <p>
            Utilizing MITM filtering could inadvertently infringe upon the
            privacy rights of individuals using the network. It is imperative to
            have a clear, legitimate purpose and legal grounds for employing
            this feature, ensuring compliance with applicable laws and
            regulations regarding data protection and privacy.
          </p>
        </li>
      </ol>

      <h4 class="section">Responsibilities:</h4>
      <ol>
        <li>
          <b>Informed Consent:</b>
          <p>
            Where applicable, obtain informed consent from network users
            regarding the use of MITM filtering and the associated privacy
            implications. Transparent communication about the use of this
            feature is essential to uphold the rights and trust of the
            individuals involved.
          </p>
        </li>
        <li>
          <b>Secure Data Handling:</b>
          <p>
            Ensure the secure and ethical handling of any accessed data.
            Implement robust security measures to protect the data from
            unauthorized access, use, or disclosure.
          </p>
        </li>
        <li>
          <b>Limited Data Access and Use:</b>
          <p>
            Limit the access and use of data obtained through MITM filtering to
            the specific, intended purpose, avoiding any unnecessary data
            processing or retention.
          </p>
        </li>
      </ol>

      <h4 class="section">Caution:</h4>
      <ol>
        <li>
          <b>Use with Discretion:</b>
          <p>
            Enable MITM filtering only when absolutely necessary and for a
            legitimate purpose. Continuously assess the need for this feature,
            keeping the potential privacy risks at the forefront of
            consideration.
          </p>
        </li>
        <li>
          <b>Legal Consultation:</b>
          <p>
            It is highly recommended to consult legal counsel to ensure the
            lawful and compliant use of MITM filtering, safeguarding against
            potential legal repercussions related to privacy infringement.
          </p>
        </li>
      </ol>

      <p>
        By toggling to enable MITM filtering, you acknowledge the serious
        privacy considerations involved and affirm your commitment to upholding
        the highest standards of privacy and data protection in the use of this
        feature.
      </p>

      <p>
        <b>Note:</b> Adjust the notice to align with your organization's policies
        and ensure it adheres to the relevant legal standards in your jurisdiction.
      </p>
    </div>
  </div>
</div>
