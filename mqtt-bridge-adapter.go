package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	cb "github.com/clearblade/Go-SDK"
	mqttTypes "github.com/clearblade/mqtt_parsing"
	mqtt "github.com/clearblade/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
)

var (
	platformURL         string //Defaults to http://localhost:9000
	messagingURL        string //Defaults to localhost:1883
	sysKey              string
	sysSec              string
	deviceName          string //Defaults to mqttBridgeAdapter
	activeKey           string
	logLevel            string //Defaults to info
	adapterConfigCollID string
	config              adapterConfig
	cbClient            *cb.DeviceClient
	cbSentMessages      SentMessages
	cbCancelCtx         context.CancelFunc
	otherCancelCtx      context.CancelFunc
	cbCtx               context.Context
	otherCtx            context.Context
)

const (
	qos = 0 // qos to use for all sub/pubs
)

type adapterConfig struct {
	BrokerConfig mqttBroker `json:"adapter_settings"`
	TopicRoot    string     `json:"topic_root"`
}

type mqttBroker struct {
	MessagingURL string   `json:"messagingURL"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	Topics       []string `json:"topics"`
	PlatformURL  string   `json:"platformURL"`
	SystemKey    string   `json:"systemKey"`
	SystemSecret string   `json:"systemSecret"`
	DeviceName   string   `json:"deviceName"`
	ActiveKey    string   `json:"activeKey"`
	IsCbBroker   bool     `json:"isCbBroker"`
	Client       mqtt.Client
}

type SentKey struct {
	Topic, Message string
}

type SentMessages struct {
	Mutex    *sync.Mutex
	Messages map[SentKey]int
}

func init() {
	flag.StringVar(&sysKey, "systemKey", "", "system key (required)")
	flag.StringVar(&sysSec, "systemSecret", "", "system secret (required)")
	flag.StringVar(&deviceName, "deviceName", "mqttBridgeAdapter", "name of device (optional)")
	flag.StringVar(&activeKey, "password", "", "password (or active key) for device authentication (required)")
	flag.StringVar(&platformURL, "platformURL", "http://localhost:9000", "platform url (optional)")
	flag.StringVar(&messagingURL, "messagingURL", "localhost:1883", "messaging URL (optional)")
	flag.StringVar(&logLevel, "logLevel", "info", "The level of logging to use. Available levels are 'debug, 'info', 'warn', 'error', 'fatal' (optional)")
	flag.StringVar(&adapterConfigCollID, "adapterConfigCollectionID", "", "The ID of the data collection used to house adapter configuration (required)")
}

func usage() {
	log.Printf("Usage: mqttBridgeAdapter [options]\n\n")
	flag.PrintDefaults()
}

func validateFlags() {
	flag.Parse()

	if sysKey == "" || sysSec == "" || activeKey == "" || adapterConfigCollID == "" {
		log.Println("ERROR - Missing required flags")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	log.Println("Starting mqttBridgeAdapter...")

	flag.Usage = usage
	validateFlags()

	rand.Seed(time.Now().UnixNano())

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer:   os.Stdout,
	}

	log.SetOutput(filter)

	cbClient = initCbClient()

	var err error

	for config.BrokerConfig.Client, err = initOtherMQTT(); err != nil; {
		log.Println("[ERROR] Failed to initialize other MQTT client, trying again in 1 minute")
		time.Sleep(time.Duration(time.Minute * 1))
		config.BrokerConfig.Client, err = initOtherMQTT()
	}

	for {
		time.Sleep(time.Duration(time.Second * 60))
		log.Println("[INFO] Listening for messages..")
	}

}

func cbMessageListener(ctx context.Context, onPubChannel <-chan *mqttTypes.Publish) {
	for {
		select {
		case message, ok := <-onPubChannel:
			if ok {
				// message published to cb broker
				if len(message.Topic.Split) >= 3 {
					log.Printf("[DEBUG] cbMessageListener - message received topic: %s message: %s\n", message.Topic.Whole, string(message.Payload))
					topicToUse := strings.Join(message.Topic.Split[2:], "/")
					cbSentMessages.Mutex.Lock()
					cbSentMessages.Messages[SentKey{topicToUse, string(message.Payload)}]++
					cbSentMessages.Mutex.Unlock()
					config.BrokerConfig.Client.Publish(topicToUse, qos, false, message.Payload)
				} else {
					log.Printf("[DEBUG] cbMessageListener - Unexpected topic for message from ClearBlade Broker: %s\n", message.Topic.Whole)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func otherMessageHandler(client mqtt.Client, msg mqtt.Message) {
	cbSentMessages.Mutex.Lock()
	n := cbSentMessages.Messages[SentKey{msg.Topic(), string(msg.Payload())}]
	if n == 1 {
		delete(cbSentMessages.Messages, SentKey{msg.Topic(), string(msg.Payload())})
		cbSentMessages.Mutex.Unlock()
		log.Println("[DEBUG] otherMessageHandler - ignoring message because it came from clearblade")
		return
	} else if n > 1 {
		cbSentMessages.Messages[SentKey{msg.Topic(), string(msg.Payload())}]--
		cbSentMessages.Mutex.Unlock()
		log.Println("[DEBUG] otherMessageHandler - ignoring message because it came from clearblade")
		return
	}
	cbSentMessages.Mutex.Unlock()
	log.Printf("[DEBUG] otherMessageHandler - message received topic: %s message: %s\n", msg.Topic(), string(msg.Payload()))
	topicToUse := config.TopicRoot + "/incoming/" + msg.Topic()
	if err := cbClient.Publish(topicToUse, msg.Payload(), qos); err != nil {
		log.Printf("[ERROR] otherMessageHandler - failed to forward message to ClearBlade: %s\n", err.Error())
	}
}

func initCbClient() *cb.DeviceClient {
	client := cb.NewDeviceClientWithAddrs(platformURL, messagingURL, sysKey, sysSec, deviceName, activeKey)

	log.Println("[INFO] initCbClient - Authenticating with ClearBlade")
	for err := client.Authenticate(); err != nil; {
		log.Printf("[ERROR] initCbClient - Error authenticating ClearBlade: %s\n", err.Error())
		log.Println("[ERROR] initCbClient - Will retry in 1 minute...")
		time.Sleep(time.Duration(time.Minute * 1))
		err = client.Authenticate()
	}

	log.Println("[INFO] initCbClient - Fetching adapter config")
	setAdapterConfig(client)

	log.Println("[INFO] initCbClient - Initializing ClearBlade MQTT")
	callbacks := cb.Callbacks{OnConnectCallback: onCBConnect, OnConnectionLostCallback: onCBDisconnect}
	if err := client.InitializeMQTTWithCallback(deviceName+"-"+strconv.Itoa(randomInt(0, 10000)), "", 30, nil, nil, &callbacks); err != nil {
		log.Fatalf("[FATAL] initCbClient - Unable to initialize MQTT connection with ClearBlade: %s", err.Error())
	}

	return client
}

func initOtherMQTT() (mqtt.Client, error) {
	log.Println("[INFO] initOtherMQTT - Initializing Other MQTT")

	if config.BrokerConfig.IsCbBroker {
		if err := initOtherCbClient(); err != nil {
			return nil, err
		}
	}

	opts := mqtt.NewClientOptions()

	opts.AddBroker(config.BrokerConfig.MessagingURL)

	if config.BrokerConfig.Username != "" {
		opts.SetUsername(config.BrokerConfig.Username)
	}

	if config.BrokerConfig.Password != "" {
		opts.SetPassword(config.BrokerConfig.Password)
	}

	opts.SetClientID(deviceName + "-" + strconv.Itoa(randomInt(0, 10000)))
	opts.SetOnConnectHandler(onOtherConnect)
	opts.SetConnectionLostHandler(onOtherDisconnect)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(false)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("[ERROR] initOtherMQTT - Unable to connect to other MQTT Broker: %s", token.Error())
		return nil, token.Error()
	}
	log.Println("[INFO] initOtherMQTT - Other MQTT Connected")
	return client, nil
}

func initOtherCbClient() error {
	client := cb.NewDeviceClientWithAddrs(config.BrokerConfig.PlatformURL,
		config.BrokerConfig.MessagingURL,
		config.BrokerConfig.SystemKey,
		config.BrokerConfig.SystemSecret,
		config.BrokerConfig.DeviceName,
		config.BrokerConfig.ActiveKey)

	log.Println("[INFO] initOtherCbClient - Authenticating with ClearBlade")
	if err := client.Authenticate(); err != nil {
		log.Printf("[ERROR] initOtherCbClient - Error authenticating ClearBlade: %s\n", err.Error())
		return err
	}
	// Set Auth username password for standard mqtt auth
	config.BrokerConfig.Username = client.DeviceToken
	config.BrokerConfig.Password = config.BrokerConfig.SystemKey
	return nil
}

func setAdapterConfig(client cb.Client) {
	log.Println("[INFO] setAdapterConfig - Fetching adapter config")

	query := cb.NewQuery()
	query.EqualTo("adapter_name", deviceName)

	log.Println("[DEBUG] setAdapterConfig - Executing query against table " + adapterConfigCollID)
	results, err := client.GetData(adapterConfigCollID, query)
	if err != nil {
		log.Fatalf("[FATAL] setAdapterConfig - Error fetching adapter config: %s", err.Error())
	}

	data := results["DATA"].([]interface{})

	if len(data) == 0 {
		log.Fatalf("[FATAL] - setAdapterConfig - No configuration found for adapter with name: %s", deviceName)
	}

	config = adapterConfig{TopicRoot: "mqtt-bridge-adapter"}

	configData := data[0].(map[string]interface{})
	log.Printf("[DEBUG] setAdapterConfig - fetched config:\n%+v\n", data)
	if configData["topic_root"] != nil {
		config.TopicRoot = configData["topic_root"].(string)
	}
	if configData["adapter_settings"] == nil {
		log.Fatalln("[FATAL] setAdapterConfig - No adapter settings required, this is required")
	}
	var bC mqttBroker
	if err := json.Unmarshal([]byte(configData["adapter_settings"].(string)), &bC); err != nil {
		log.Fatalf("[FATAL] setAdapterConfig - Failed to parse adapter_settings: %s", err.Error())
	}

	config.BrokerConfig = bC

	if config.BrokerConfig.MessagingURL == "" {
		log.Fatalln("[FATAL] setAdapterConfig - No messaging URL defined for broker config adapter_settings")
	}

	log.Printf("[DEBUG] setAdapterConfig - Using adapter settings:\n%+v\n", config)
}

func onCBConnect(client mqtt.Client) {
	log.Println("[DEBUG] onCBConnect - ClearBlade MQTT connected")

	// subscribe
	//on cb we subscribe to all outgoing topics prefaced with topic root
	var err error
	log.Println("[INFO] Subscribing to outgoing clearblade topic")
	var cbSubChannel <-chan *mqttTypes.Publish
	for cbSubChannel, err = cbClient.Subscribe(config.TopicRoot+"/outgoing/#", qos); err != nil; {
	}
	// listen
	cbCtx, cbCancelCtx = context.WithCancel(context.Background())
	go cbMessageListener(cbCtx, cbSubChannel)
}

func onCBDisconnect(client mqtt.Client, err error) {
	log.Printf("[DEBUG] onCBDisonnect - ClearBlade MQTT disconnected: %s", err.Error())
	cbCancelCtx()
}

func onOtherConnect(client mqtt.Client) {
	log.Println("[DEBUG] onOtherConnect - Other MQTT connected")
	//create map that stores sent messages, need this because we have no control of topic structure on other MQTT broker,
	// so we can't break messages out into incoming/outgoing topics like the clearblade side does
	cbSentMessages = SentMessages{
		Mutex:    &sync.Mutex{},
		Messages: make(map[SentKey]int),
	}

	//on other mqtt we subscribe to the provided topics, or all topics if nothing is provided
	if len(config.BrokerConfig.Topics) == 0 {
		log.Println("[INFO] No topics provided, subscribing to all topics for other MQTT broker")
		config.BrokerConfig.Client.Subscribe("#", qos, otherMessageHandler)
	} else {
		log.Printf("[INFO] Subscribing to remote topics: %+v\n", config.BrokerConfig.Topics)
		for _, element := range config.BrokerConfig.Topics {
			config.BrokerConfig.Client.Subscribe(element, qos, otherMessageHandler)
		}
	}
}

func onOtherDisconnect(client mqtt.Client, err error) {
	log.Printf("[DEBUG] onOtherConnect - Other MQTT disconnected: %s", err.Error())
}

func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}
