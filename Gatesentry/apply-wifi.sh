#!/bin/bash

i=1;
FILE=$1
k=1

if [ -f "$FILE" ];
then
while read line;do
	if [ $k == 1 ]
		then
			AP="$line"
	fi
	if [ $k == 2 ]
		then
			APPass="$line"
	fi
        ((k++))
done < $FILE
echo "File read"
echo "New AP: '$AP' AND '$APPass'"
echo "Applying Changes"
config="interface=wlan0
driver=nl80211
ssid=$AP
hw_mode=g
channel=6
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
wpa=2
wpa_passphrase=$APPass
wpa_key_mgmt=WPA-PSK
wpa_pairwise=TKIP
rsn_pairwise=CCMP"
`echo "$config" > /tmp/newhostapd.conf`
sudo rm /etc/hostapd/hostapd.conf
#sudo mv /etc/hostapd/hostapd.conf /etc/hostapd/hostapd.conf.old
sudo mv /tmp/newhostapd.conf /etc/hostapd/hostapd.conf
sudo service hostapd restart
rm $FILE
sudo reboot
else
echo "Config File not found"
fi
