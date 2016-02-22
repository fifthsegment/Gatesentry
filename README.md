
##GateSentry
GateSentry is a complete Web Filtering suite for the Raspberry Pi that supports both HTTP and HTTPS filtering. It can be downloaded and used as a SD-CARD image from : www.abdullahirfan.com/my-projects/gatesentry/.

### Under the hood
GateSentry is powered by:
* Squid3 compiled with sslbump
* Dansguardian for http filtering 
* GateSentry's ICAP server for https filtering
* PHP5 and Sqlite for the Administration panel

### Compiling and Running

####ICAP Server
GateSentry's source uses the Python 2.7 interpreter.

To Run:

`python icap_server.py [path of .cfg file here]`

Example:

`python icap_server.py icap_server.cfg`

####Administration Panel


The admin panel is powered by Laravel, so you'll need a webserver to run it. On the Raspberry Pi  GateSentry uses Lighttpd to serve the administration panel.

Just provide the following path as your document Root to Lighttpd:

 `<base_path>/Gatesentry/admin_panel/site/public/`

 ####Squid3 

 GateSentry uses Squid3 with -sslbump enabled. GateSentry's Squid3 config file can be found in the Squid3 labelled folder of this repo.

 ####Dansguardian

 Config file available in the repo. 

 ###Running GateSentry

 Once everything is in place (Squid3, Dansguardian, GateSentry's ICAP server and the Lighttpd). Start services in the following order :

 1-Lighttpd
 2-Squid3
 3-Dansguardian
 4-GateSentry



