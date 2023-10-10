<div class="bx--row">
  <div class="bx--col">
    <h4>Google Chrome:</h4>
    <br />
    <ol>
      <li>
        <h5>Open Chrome Settings:</h5>
        <ul>
          <li>
            Click on the three vertical dots in the top-right corner of the
            browser window.
          </li>
          <li>Select Settings from the drop-down menu.</li>
        </ul>
      </li>
      <li>
        <h5>Access Advanced Settings:</h5>
        <ul>
          <li>
            Scroll down to the bottom of the settings page and click on
            Advanced.
          </li>
        </ul>
      </li>
      <li>
        <h5>Change Proxy Settings:</h5>
        <ul>
          <li>
            In the System section, click on Open your computer’s proxy settings.
          </li>
        </ul>
      </li>
      <li>
        <h5>Set Up Proxy:</h5>
        <ul>
          <li>
            Under the Manual proxy setup section, toggle on Use a proxy server.
          </li>
          <li>
            In the Address field, enter Gatesentry Server IP and in the Port
            field, enter 10786.
          </li>
          <li>Click Save or OK to apply the changes.</li>
        </ul>
      </li>
    </ol>
  </div>
  <div class="bx--col">
    <h4>Mozilla Firefox:</h4>
    <br />
    <ol>
      <li>
        <h5>Open Firefox Options:</h5>
        <ul>
          <li>
            Click on the three horizontal lines in the top-right corner of the
            browser window.
          </li>
          <li>Select Options from the drop-down menu.</li>
        </ul>
      </li>
      <li>
        <h5>Navigate to Network Settings:</h5>
        <ul>
          <li>
            Scroll down to the Network Settings section and click on Settings.
          </li>
        </ul>
      </li>
      <li>
        <h5>Configure Proxy:</h5>
        <ul>
          <li>Select Manual proxy configuration.</li>
          <li>
            In the HTTP Proxy field, enter Gatesentry Server IP and in the Port
            field, enter 10786.
          </li>
          <li>
            Optionally, check the box for Use this proxy server for all
            protocols.
          </li>
          <li>Click OK to save the changes.</li>
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
    <div>
      <h4>Mac:</h4>
      <ol>
        <li>
          <h5>Open Keychain Access:</h5>
          <ul>
            <li>Go to Applications -> Utilities -> Keychain Access.</li>
          </ul>
        </li>
        <li>
          <h5>Import Certificate:</h5>
          <ul>
            <li>
              Go to File -> Import Items, locate your certificate file, and
              click Open.
            </li>
          </ul>
        </li>
        <li>
          <h5>Trust Certificate:</h5>
          <ul>
            <li>
              Find and select your certificate in Keychain Access, right-click,
              choose Get Info.
            </li>
            <li>
              Expand the Trust section, and in the When using this certificate
              select Always Trust.
            </li>
          </ul>
        </li>
      </ol>
    </div>
    <br />
    <div>
      <h4>Windows:</h4>
      <ol>
        <li>
          <h5>Open Certificate Manager:</h5>
          <ul>
            <li>Type “certmgr.msc” in the Start menu, and press Enter.</li>
          </ul>
        </li>
        <li>
          <h5>Import Certificate:</h5>
          <ul>
            <li>
              Right-click on the Trusted Root Certification Authorities folder,
              choose All Tasks -> Import.
            </li>
            <li>
              Follow the Certificate Import Wizard, locate your certificate
              file, and complete the wizard.
            </li>
          </ul>
        </li>
      </ol>
    </div>
    <br />
    <div>
      <h4>Linux (Ubuntu):</h4>
      <ol>
        <li>
          <h5>Copy Certificate File:</h5>
          <ul>
            <li>
              Copy your certificate file into the
              /usr/local/share/ca-certificates/ directory.
            </li>
          </ul>
        </li>
        <li>
          <h5>Update Certificates:</h5>
          <ul>
            <li>
              Open a terminal and run the following command: sudo
              update-ca-certificates
            </li>
          </ul>
        </li>
      </ol>
    </div>
  </div>
</div>
