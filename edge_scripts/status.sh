#!/bin/bash

# If this script is executed, we know the adapter has been deployed. No need to test for that.
STATUS="Deployed"
if [[ $(ps aux | grep -v grep | grep mqttBridgeAdapter) ]]; then
    STATUS="Running"
else
    STATUS="Stopped"
fi

echo $STATUS