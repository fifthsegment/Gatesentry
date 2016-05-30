echo "Copying systemd service"
sudo cp /etc/gatesentry/gatesentry.service /lib/systemd/system/
sudo systemctl daemon-reload
cd /etc/systemd/system/
ln -s /lib/systemd/system/gatesentry.service gatesentry.service
sudo systemctl daemon-reload
sudo systemctl start gatesentry.service
sudo systemctl enable gatesentry.service
echo "Install php5"
sudo apt-get install lighttpd php5 php5-cgi php5-common php-pear php5-sqlite php5-dev
echo "Extracting admin panel"
cd /etc/gatesentry
sudo unzip /etc/gatesentry/admin_panel.zip
echo "Setting permissions"
sudo chmod 777 /etc/gatesentry/admin_panel
sudo chown -R www-data:www-data /etc/gatesentry/admin_panel
echo "Installing server"
sudo apt-get install lighttpd
sudo apt-get install php5-mcrypt
lighty-enable-mod fastcgi
lighty-enable-mod fastcgi-php
service lighttpd force-reload


