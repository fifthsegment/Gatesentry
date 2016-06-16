
## GateSentry
##### *A Free webfilter + Parental Controls Suite for the Pi*




GateSentry is a complete Web Filtering suite for the Raspberry Pi that supports both HTTP and HTTPS filtering. It can be downloaded and used as simply as a SD-CARD image .

##### Features
* SSL Filtering
* File Download restriction based upon MIME types
* Phrase based content restriction
* Individual Site blocking
* Ad Blocking
* Switch Internet access on or off.
* Updated Squid
* Support for all Raspberry Pi boards upto Raspberry Pi 3
* Built in Wifi Access point for the Pi3
* Sets up proxy automatically on clients using WPAD, works out of the Box on the Pi3 and Pi2 (if * your Wifi Device is supported)
* Support for OTA updates
* Its Free!

##### Screenshots
1  - Main Screen
![Main-Screen](http://i.imgur.com/oB5FiBL.png)
2  - Change built-in Wifi name and password
![Wifi-pass](http://i.imgur.com/sE4ev7c.png)
3 - Disable Internet access for Wifi Clients
![Disable-access](http://i.imgur.com/DNYDmrG.png)
4 - Edit Filters
![Edit-filters](http://i.imgur.com/8XyoPJs.png)

##### Download and Initial Setup Guide

[https://www.abdullahirfan.com/releasing-gatesentry-v1-0-beta/](Here)

##### Using a self-signed certificate


Even though GateSentry comes with its own certificate, for security purposes you're encouraged to generate your own. Here's how:


### Under the hood
GateSentry is powered by:
* Squid3 compiled with sslbump
* Dansguardian for http filtering 
* GateSentry's ICAP server for https filtering
* PHP5 and Sqlite powered by Laravel (for the Administration panel)

### Compiling and Running

####ICAP Server
GateSentry's source uses the Python 2.7 interpreter.

To Run:

`python icap_server.py [path of .cfg file here]`

Example:

`python icap_server.py icap_server.cfg`

#### Administration Panel

The admin panel is powered by Laravel, so you'll need a webserver to run it. On the Raspberry Pi  GateSentry uses Lighttpd to serve the administration panel.

Just provide the following path as your document Root to Lighttpd:

 `<base_path>/Gatesentry/admin_panel/site/public/`

#### Squid3 

 GateSentry uses Squid3 with -sslbump enabled. GateSentry's Squid3 config file can be found in the Squid3 labelled folder of this repo.

####Dansguardian

 Config file available in the repo. 

###Running GateSentry

Once everything is in place (Squid3, Dansguardian, GateSentry's ICAP server and the Lighttpd). Start services in the following order :

 1. Lighttpd
 2. Squid3
 3. Dansguardian
 4. GateSentry



