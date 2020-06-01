# MQTT Bridge Adapter

The __mqttBridgeAdapter__ adapter provides the ability for the ClearBlade Edge or Platform to interface with any other MQTT broker. This includes subscribing to any topics (including wildcards) on the external MQTT broker and forwarding any received messages to the ClearBlade MQTT broker, as well as forwarding messages received on the ClearBlade MQTT broker to the external MQTT broker.

The adapter forwards along the MQTT message as is, and keeps the full topic structure the original message was published on, allowing you to access any relevent data in the message as well as the topic. 

## MQTT Topic Structure
### Sending Messages to External MQTT Broker
In order to forward any messages from the ClearBlade Edge or Platform MQTT broker, the adapter will subscribe to a specific topic using a wildcard. The default topic is `{TOPIC ROOT}/outgoing/#`, where __{TOPIC ROOT}__ is replaced with the provided topic root in the adapter configuration collection. If no root topic is provided, a default of `mqtt-bridge-adapter` will be used.

To send a message to the external MQTT broker you simply need to publish a message on a topic prefaced with the above mentioned topic. The adapter will then trim the `{TOPIC ROOT}/outgoing` level of the topic, and forward the message as is to the external MQTT broker.

For example, if you want to publish a message to the topic `lora/abc123/down` on the external MQTT broker, you will publish a message to the ClearBlade Edge or Platform (whichever the adapter is pointed at) using the topic `{TOPIC ROOT}/outgoing/lora/abc123/down`.

### Receiving Messages from External MQTT Broker
You must provide the mqttBridgeAdapter the topics to subscribe to on the external MQTT broker. This is done using the adapter_settings object provided via the adapter configuration collection. You simply provide an array of topics (wildcards are supported) to subscribe to. After the adapter successfully connects to the external MQTT broker it will subscribe to all provided topics, and begin forwarding any received messages.

Messages received from the external MQTT broker will be prefaced with a specific topic before being published into the ClearBlade MQTT Broker. This preface will be `{TOPIC ROOT}/incoming`, and appended to the end of this will be the specific topic the message was published on.

For example, if you provide the topic `lora/+/up` in your adapter_settings, and a message is received on this topic. The adapter will publish this message to the ClearBlade MQTT Broker on the topic `{TOPIC_ROOT}/incoming/lora/abc123/up`.


## MQTT Payloads
This adapter will just forward along the provided message payload, so there is no specific payload format required.


## ClearBlade Platform Dependencies
The mqttBridgeAdapter adapter was constructed to provide the ability to communicate with a _System_ defined in a ClearBlade Platform instance. Therefore, the adapter requires a _System_ to have been created within a ClearBlade Platform instance.

Once a System has been created, artifacts must be defined within the ClearBlade Platform system to allow the adapters to function properly. At a minimum: 

  * A device needs to be created in the Auth --> Devices collection. The device will represent the adapter, or more importantly, the device or gateway on which the adapter is executing. The _name_ and _active key_ values specified in the Auth --> Devices collection will be used by the adapter to authenticate to the ClearBlade Platform or ClearBlade Edge. 
  * This device must have a role assigned to it that at minimum has these permissions:
     * Publish and Subscribe to `{TOPIC_ROOT}/incoming/#` and `{TOPIC_ROOT}/outgoing/#`
     * Read on the adapter configuration collection 
  * An adapter configuration data collection needs to be created in the ClearBlade Platform _system_ and populated with the data appropriate to the mqtt-bridge adapter. The schema of the data collection should be as follows:

| Column Name      | Column Datatype |
| ---------------- | --------------- |
| adapter_name     | string          |
| topic_root       | string          |
| adapter_settings | string (json)   |

### adapter_settings
An adapter_settings object is required for this adapter to function. See below for all accepted options on this object

| Key              | Value           |
| ---------------- | --------------- |
| messagingURL (__required__) | URL of the external MQTT broker (expected format is `tcp://{ip or host name}:{port}`) |
| username (_optional_) | Username to use when connecting to external MQTT broker, can be ommited if no username is required |
| password (_optional_) | Password to use when connecting to external MQTT broker, can be ommited if no password is required |
| topics (__required__) | An array of strings that the adapter should subscribe to on the external MQTT broker |
| isCbBroker (default=false) | Let's the adapter know if the Broker to connect to is a ClearBlade Broker or not|
|platformURL (required if `isCbBroker`=true) | URL of the ClearBlade Platform to Authenticate with|
|systemKey (required if `isCbBroker`=true) | SystemKey of the ClearBlade System which user is connecting to |
| systemSecret  (required if `isCbBroker`=true) | SystemSecret of the ClearBlade System which user is connecting to |
| deviceName  (required if `isCbBroker`=true) |DeviceName of the device client which subscribes to the external MQTT broker |
| activeKey (required if `isCbBroker`=true)| ActiveKey of the device client which subscribes to the external MQTT broker |

Here is an example adapter_settings object where the external MQTT broker is running on the same gateway as the adapter, on port 1883, does not require any authentication, and we want to subscribe to only the `lora/+/up` topic:

```
{
  "messagingURL": "tcp://localhost:1883",
  "topics": [
    "lora/+/up"
  ]
}
```

## Usage
In the `edge_scripts` directory of this repo we have provided example scripts, including an init.d service configuration for running this adapter on a Multitech Gateway. If you plan on running on other gateways, some modifications of these scripts will be required.

### Starting the adapter
The full command to start the adapter is as follows:

`mqttBridgeAdapter -systemKey=<SYSTEM_KEY> -systemSecret=<SYSTEM_SECRET> -platformURL=<PLATFORM_URL> -messagingURL=<MESSAGING_URL> -deviceName=<DEVICE_NAME> -password=<DEVICE_ACTIVE_KEY> -adapterConfigCollectionID=<COLLECTION_ID> -logLevel=<LOG_LEVEL>`

 __*Where*__ 

   __systemKey__
  * REQUIRED
  * The system key of the ClearBlade Platform __System__ the adapter will connect to

   __systemSecret__
  * REQUIRED
  * The system secret of the ClearBlade Platform __System__ the adapter will connect to
   
   __deviceName__
  * The device name the adapter will use to authenticate to the ClearBlade Platform
  * Requires the device to have been defined in the _Auth - Devices_ collection within the ClearBlade Platform __System__
  * OPTIONAL
  * Defaults to __mqttBridgeAdapter__
   
   __password__
  * REQUIRED
  * The active key the adapter will use to authenticate to the platform
  * Requires the device to have been defined in the _Auth - Devices_ collection within the ClearBlade Platform __System__
   
   __platformUrl__
  * The url of the ClearBlade Platform instance the adapter will connect to
  * OPTIONAL
  * Defaults to __http://localhost:9000__

   __messagingUrl__
  * The MQTT url of the ClearBlade Platform instance the adapter will connect to
  * OPTIONAL
  * Defaults to __localhost:1883__

   __adapterConfigCollectionID__
  * REQUIRED 
  * The collection ID of the data collection used to house adapter configuration data

   __logLevel__
  * The level of runtime logging the adapter should provide.
  * Available log levels:
    * fatal
    * error
    * warn
    * info
    * debug
  * OPTIONAL
  * Defaults to __info__


## Development
The mqtt-bridge-adapter adapter is dependent upon the ClearBlade Go SDK and its dependent libraries being installed. The mqtt-bridge-adapter adapter was written in Go and therefore requires Go to be installed (https://golang.org/doc/install).

### Adapter compilation
In order to compile the adapter for execution within mLinux, the following steps need to be performed:

 1. Retrieve the adapter source code  
    * ```git clone git@github.com:ClearBlade/mqtt-bridge-adater.git```
 2. Navigate to the _mqtt-bridge-adapter_ directory  
    * ```cd MTAC-GPIO-ADAPTER```
 3. Compile the adapter
    * ```GOARCH=arm GOARM=5 GOOS=linux go build -o mtacGpioAdapter```
