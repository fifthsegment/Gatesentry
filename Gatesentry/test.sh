#!/bin/bash
FILE="/tmp/gatesentry-restart"


if [ -f "$FILE" ];
  then
echo " Restarting"
  else
echo " Clear"
fi
