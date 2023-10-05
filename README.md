# Gatesentry

A DNS + proxy server (supports MITM) with a nice frontend.

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

    `service GateSentry start   #starts the service`

    `service GateSentry stop    #stops the service`

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
