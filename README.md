# Gatesentry

An open source proxy server (supports SSL filtering / MITM) + DNS Server with a nice frontend.

![Codecov](https://codecov.io/gh/fifthsegment/Gatesentry/branch/master/graph/badge.svg)


[Download the latest release](https://github.com/fifthsegment/Gatesentry/releases)

## 🚀 Optimized for Low-Spec Hardware

Gatesentry is now optimized to run efficiently on routers and embedded devices with limited resources!

**See [ROUTER_OPTIMIZATION.md](ROUTER_OPTIMIZATION.md) for:**
- Performance optimization details
- Configuration options for different hardware specs
- Recommended settings for routers with 64MB - 1GB RAM
- Troubleshooting tips

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

There are 2 ways to run Gatesentry, either using the docker image or using the single file binary directly. 

### Method 1: Using Docker

1. Use the [docker-compose.yml](https://github.com/fifthsegment/Gatesentry/blob/master/docker-compose.yml) file from the root of this repo as a template, copy and paste it to any directory on your computer, then run the following command in a terminal `docker compose up`

### Method 2: Using the Gatesentry binary directly

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

    **Running as a Service (Optional)**

    If you want Gatesentry to keep running in the background on your machine, install it as :

    `./gatesentry-{platform} -service install`

    Next, on linux you can use your system service runner to start or stop it, for example for ubuntu:

    `service gatesentry start   #starts the service`

    `service gatesentry stop    #stops the service`

    **For Windows**

    The installer (GatesentrySetup.exe) contains instructions.

    **Running as a Service**

    The installer (GatesentrySetup.exe) should automatically install a service. You can look for it by searching for gatesentry in your Service manager (open it by running `services.msc`)

3.  Launching the Server:

    Execute the Gatesentry binary file to start the server.
    Upon successful launch, the server will begin listening for incoming connections on port 10413.

## Important information

### Ports

By default Gatesentry uses the following ports

| Port  | Purpose                                              |
| ----- | ---------------------------------------------------- |
| 10413 | For proxy                                            |
| 10786 | For the web based administration panel               |
| 53    | For the built-in DNS server                          |
| 80    | For the built-in webserver (showing DNS block pages) |

### Accessing the User Interface:

Open a modern web browser of your choice.
Enter the following URL in the address bar: http://localhost:10786
The Gatesentry User Interface will load, providing access to various functionalities and settings.

### Default Login Credentials:

    Username: admin
    Password: admin

Use the above credentials to log in to the Gatesentry system for the first time. For security reasons, it is highly recommended to change the default password after the initial login.

Note:Ensure your system’s firewall and security settings allow traffic on ports 10413 and 10786 to ensure seamless operation and access to the Gatesentry server and user interface.

This guide now specifically refers to the Gatesentry software and uses the `gatesentry-{platform}` filename convention for clarity.

### DNS Information

Gatesentry ships with a built in DNS server, which can be used to block domains. The server as of now forwards requests to Google DNS for resolution, this can be modified from inside the `application/dns/server/server.go` file. 

## Local Development

`./setup.sh`

To run it:

`./run.sh`
