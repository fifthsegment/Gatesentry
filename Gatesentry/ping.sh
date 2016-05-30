#!/bin/bash
SERIAL="$(cat /proc/cpuinfo | grep Serial | cut -d ':' -f 2)"
#echo $SERIAL
/usr/bin/curl --data "id=$SERIAL" http://api.fifthsegment.com/v1/GateSentry/hello > /tmp/gatesentry-hello
rm /tmp/gatesentry-hello
