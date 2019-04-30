#!/bin/bash

#Stop the adapter
monit stop mqttBridgeAdapter

#Remove xDotAdapter from monit
sed -i '/mqttBridgeAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Remove the init.d script
rm /etc/init.d/mqttBridgeAdapter

#Remove the default variables file
rm /etc/default/mqttBridgeAdapter

#Remove the adapter log file from log rotate
rm /etc/logrotate.d/mqttBridgeAdapter.conf

#Remove the binary
rm /usr/bin/mqttBridgeAdapter

#reload monit config
monit reload