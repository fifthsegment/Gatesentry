###THIS REPO IS BEING UPDATED

##GateSentry
GateSentry is a complete Web Filtering suite for the Raspberry Pi that supports both HTTP and HTTPS filtering. It can be downloaded and used as a SD-CARD image from : www.abdullahirfan.com/my-projects/gatesentry/.

### Under the hood
GateSentry is powered by:
* Squid3 compiled with sslbump
* Dansguardian for http filtering 
* GateSentry's ICAP server for https filtering
* PHP5 and Sqlite for the Administration panel

### Compiling and Running
GateSentry's source uses the Python 2.7 interpreter.

To Run:

`python icap_server.py [path of .cfg file here]`

Example:

`python icap_server.py icap_server.cfg`



