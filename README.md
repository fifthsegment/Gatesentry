# Gatesentry

An open source proxy server (supports SSL filtering / MITM) + DNS Server with a nice frontend.

Usages:

- Privacy Protection: Users can use Gatesentry to prevent tracking by various online services by blocking tracking scripts and cookies.

- Parental Controls: Parents can configure Gatesentry to block inappropriate content or websites for younger users on the network.

- Bandwidth Management: By blocking unnecessary content like ads or heavy scripts, users can save on bandwidth, which is especially useful for limited data plans.

- Enhanced Security: Gatesentry can be used to block known malicious websites or phishing domains, adding an extra layer of security to the network.

- Access Control: In a corporate or institutional setting, Gatesentry can be used to restrict access to non-work-related sites during work hours.

- Logging and Monitoring: Track and monitor all the requests made in the network to keep an eye on suspicious activities or to analyze network usage patterns.

- Custom Redirects (via DNS): Redirect specific URLs to other addresses, useful for local development or for redirecting deprecated domains.

![gatesentry-repo](https://github.com/fifthsegment/Gatesentry/assets/5513549/5ab836ab-7362-4916-9f7c-655e67e4deab)

## Getting started

Gatesentry Installation and Configuration Guide

1.  Downloading Gatesentry:

    Navigate to the 'Releases' section of this repository.
    Identify and download the appropriate file for your operating system, named either gatesentry-linux or gatesentry-mac.

2.  Installation:

    **For macOS and Linux:**

    Locate the downloaded Gatesentry binary file in your system.
    Open a terminal window and navigate to the directory containing the downloaded binary.
    Run the following command to grant execution permissions to the binary file:

        chmod +x gatesentry-{platform}

    Replace `{platform}` with your operating system (linux or mac).
    Proceed to execute the binary file to initiate the server.

    **Running as a Service**

    If you want Gatesentry to keep running in the background on your machine, install it as :

    `./gatesentry-{platform} -service install`

    Next, on linux you can use your system service runner to start or stop it, for example for ubuntu:

    `service gatesentry start   #starts the service`

    `service gatesentry stop    #stops the service`

    **For Windows**

    Simply run the binary on your system.

    **Running as a Service**

    If you want Gatesentry to keep running in the background on your machine, install it as :

    `./gatesentry-{platform} -service install`

    Then you can use the `services.msc` in Windows to start or stop Gatesentry (Look for a service named `GateSentry` in the services.msc UI)

3.  Launching the Server:

    Execute the Gatesentry binary file to start the server.
    Upon successful launch, the server will begin listening for incoming connections on port 10413.

4.  Accessing the User Interface:

    Open a modern web browser of your choice.
    Enter the following URL in the address bar: http://localhost:10786
    The Gatesentry User Interface will load, providing access to various functionalities and settings.

5.  Default Login Credentials:

    Username: admin
    Password: admin

    Use the above credentials to log in to the Gatesentry system for the first time. For security reasons, it is highly recommended to change the default password after the initial login.

    Note:Ensure your system’s firewall and security settings allow traffic on ports 10413 and 10786 to ensure seamless operation and access to the Gatesentry server and user interface.

    This guide now specifically refers to the Gatesentry software and uses the `gatesentry-{platform}` filename convention for clarity.

## Build

`./setup.sh`

Run

`./run.sh`
