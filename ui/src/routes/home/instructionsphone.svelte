<div class="bx--row">
  <div class="bx--col">
    <h4>iOS:</h4>
    <br />
    <ol>
      <li>
        <h5>Open Settings:</h5>
        <ul>
          <li>Tap on the Settings icon from your Home screen.</li>
        </ul>
      </li>
      <li>
        <h5>Select Wi-Fi:</h5>
        <ul>
          <li>Tap on Wi-Fi and select the network you are connected to.</li>
        </ul>
      </li>
      <li>
        <h5>Configure Proxy:</h5>
        <ul>
          <li>Scroll down and tap on the 'Configure Proxy' option.</li>
          <li>
            Select 'Manual' and enter 'Gatesentry Server IP' in the Server
            field, and '10786' in the Port field.
          </li>
          <li>Tap 'Save' to apply the settings.</li>
        </ul>
      </li>
    </ol>
  </div>

  <div class="bx--col">
    <h4>Android:</h4>
    <br />
    <ol>
      <li>
        <h5>Open Settings:</h5>
        <ul>
          <li>
            Swipe down from the top of the screen and tap on the gear icon to
            open Settings.
          </li>
        </ul>
      </li>
      <li>
        <h5>Select Network & Internet:</h5>
        <ul>
          <li>
            Tap on 'Network & Internet', then 'Wi-Fi', and then on the network
            you are connected to.
          </li>
        </ul>
      </li>
      <li>
        <h5>Configure Proxy:</h5>
        <ul>
          <li>Scroll down and tap on 'Advanced' or 'Modify Network'.</li>
          <li>
            Select 'Manual' under Proxy options, and enter 'Gatesentry Server
            IP' in the Proxy hostname field, and '10786' in the Proxy port
            field.
          </li>
          <li>Tap 'Save' or 'OK' to apply the settings.</li>
        </ul>
      </li>
    </ol>
  </div>
</div>
<br />
<h4 class="">2. Install the Root Certificate</h4>
<p>This step is only required if you have MITM filtering enabled.</p>
<br />
<div class="bx--row">
  <div class="bx--col">
    <h4>iOS:</h4>
    <br />
    <ol>
      <li>
        <h5>Transfer Certificate File:</h5>
        <ul>
          <li>
            Transfer the certificate file (.cer or .crt) to your device via
            email, AirDrop, or other methods.
          </li>
        </ul>
      </li>
      <li>
        <h5>Install Certificate:</h5>
        <ul>
          <li>
            Tap on the certificate file and follow the on-screen instructions to
            install the certificate.
          </li>
        </ul>
      </li>
      <li>
        <h5>Trust Certificate:</h5>
        <ul>
          <li>
            Go to Settings -> General -> About -> Certificate Trust Settings.
          </li>
          <li>
            Under "Enable full trust for root certificates," toggle on the trust
            setting for the installed certificate.
          </li>
        </ul>
      </li>
    </ol>
  </div>
  <div class="bx--col">
    <h4>Android:</h4>
    <br />
    <ol>
      <li>
        <h5>Transfer Certificate File:</h5>
        <ul>
          <li>
            Transfer the certificate file (.cer or .crt) to your device via
            email, Bluetooth, or other methods.
          </li>
        </ul>
      </li>
      <li>
        <h5>Install Certificate:</h5>
        <ul>
          <li>
            Go to Settings -> Security & location -> Encryption & credentials ->
            Install from storage.
          </li>
          <li>
            Locate and tap on the certificate file, then follow the on-screen
            instructions to install the certificate.
          </li>
        </ul>
      </li>
      <li>
        <h5>Name and Use Certificate:</h5>
        <ul>
          <li>
            You will be prompted to name the certificate and choose its use,
            such as VPN and apps or Wi-Fi.
          </li>
          <li>
            Enter the desired name and select the intended use, then tap OK.
          </li>
        </ul>
      </li>
    </ol>
  </div>
</div>
