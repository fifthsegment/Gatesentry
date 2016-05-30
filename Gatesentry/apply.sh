#!/bin/bash
FILE="/tmp/gatesentry-restart"
INTERNETSWITCHon="/tmp/gatesentry-internet-on"
INTERNETSWITCHoff="/tmp/gatesentry-internet-off"
if [ -f "$FILE" ];
then
   echo "Reload File $FILE exists.Restarting."
   sudo service gatesentry restart
   sudo service squid3 restart
   sudo service dansguardian restart
   sudo rm $FILE
else
   echo "Reload File $FILE does not exist" >&2
fi
if [ -f "$INTERNETSWITCHon" ];
then
	sudo service squid3 start
	sudo rm $INTERNETSWITCHon
else
	echo "SWITCHON File not found" >&2
fi
if [ -f "$INTERNETSWITCHoff" ];
then
        sudo service squid3 stop
        sudo rm $INTERNETSWITCHoff
else
        echo "SWITCHOFF File not found" >&2
fi

