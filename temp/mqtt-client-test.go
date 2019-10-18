package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	cb "github.com/clearblade/Go-SDK"

	mqtt "github.com/clearblade/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
)

var (
	platformURL    string //Defaults to http://localhost:9000
	messagingURL   string //Defaults to localhost:1883
	sysKey         string
	sysSec         string
	deviceName     string //Defaults to mqttBridgeAdapter
	activeKey      string
	logLevel       string //Defaults to info
	cbClient       *cb.DeviceClient
	cbSentMessages SentMessages
	cbCancelCtx    context.CancelFunc
	otherCancelCtx context.CancelFunc
	cbCtx          context.Context
	otherCtx       context.Context
	clientConfig   mqttBroker
	topic          string
)

const (
	qos = 0 // qos to use for all sub/pubs
)

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
	flag.StringVar(&topic, "topic", "abc", "some topic name")
	flag.StringVar(&logLevel, "logLevel", "info", "The level of logging to use. Available levels are 'debug, 'info', 'warn', 'error', 'fatal' (optional)")
}

func usage() {
	log.Printf("Usage: mqttBridgeAdapter [options]\n\n")
	flag.PrintDefaults()
}

func validateFlags() {
	flag.Parse()

	if sysKey == "" || sysSec == "" || activeKey == "" {
		log.Println("ERROR - Missing required flags")
		flag.Usage()
		os.Exit(1)
	}
}

var BuildId string = "unset"

func main() {
	log.Printf("Starting mqttBridgeAdapter... BuildId: %s", BuildId)

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

	cbClient = cb.NewDeviceClientWithAddrs(platformURL, messagingURL, sysKey, sysSec, deviceName, activeKey)

	log.Println("[INFO] initCbClient - Authenticating with ClearBlade")
	for err := cbClient.Authenticate(); err != nil; {
		log.Printf("[ERROR] initCbClient - Error authenticating ClearBlade: %s\n", err.Error())
		log.Println("[ERROR] initCbClient - Will retry in 1 minute...")
		time.Sleep(time.Duration(time.Minute * 1))
		err = cbClient.Authenticate()
	}

	token := cbClient.DeviceToken

	clientConfig.MessagingURL = messagingURL
	clientConfig.Username = token
	clientConfig.Password = sysKey
	clientConfig.Topics = append(clientConfig.Topics, topic)

	var err error

	for err = initOtherMQTT(); err != nil; {
		log.Println("[ERROR] Failed to initialize other MQTT client, trying again in 1 minute")
		time.Sleep(time.Duration(time.Minute * 1))
		err = initOtherMQTT()
	}

	// for {
	// 	time.Sleep(time.Duration(time.Second * 60))
	// 	log.Println("[INFO] Listening for messages..")
	// }
	c := make(chan struct{})
	<-c
}

func otherMessageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("[DEBUG] otherMessageHandler - message received topic: %s message: %s\n", msg.Topic(), string(msg.Payload()))

}

func initOtherMQTT() error {
	log.Println("[INFO] initOtherMQTT - Initializing Other MQTT", clientConfig)

	opts := mqtt.NewClientOptions()

	opts.AddBroker(clientConfig.MessagingURL)

	if clientConfig.Username != "" {
		opts.SetUsername(clientConfig.Username)
	}

	if clientConfig.Password != "" {
		opts.SetPassword(clientConfig.Password)
	}

	opts.SetClientID(deviceName + "-" + "123")
	opts.SetOnConnectHandler(onOtherConnect)
	opts.SetConnectionLostHandler(onOtherDisconnect)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetKeepAlive(6 * time.Second)
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("[ERROR] initOtherMQTT - Unable to connect to other MQTT Broker: %s", token.Error())
		return token.Error()
	}
	log.Println("[INFO] initOtherMQTT - Other MQTT Connected")
	return nil
}

func onOtherConnect(client mqtt.Client) {
	log.Println("[DEBUG] onOtherConnect - Other MQTT connected")
	// Reset the OtherBroker Client on Reconnect
	clientConfig.Client = client
	//on other mqtt we subscribe to the provided topics, or all topics if nothing is provided
	if len(clientConfig.Topics) == 0 {
		log.Println("[INFO] No topics provided, subscribing to all topics for other MQTT broker")
		clientConfig.Client.Subscribe("#", qos, otherMessageHandler)
	} else {
		log.Printf("[INFO] Subscribing to remote topics: %+v\n", clientConfig.Topics)
		for _, element := range clientConfig.Topics {
			clientConfig.Client.Subscribe(element, qos, otherMessageHandler)
		}
	}

}

func onOtherDisconnect(client mqtt.Client, err error) {
	log.Printf("[DEBUG] onOtherConnect - Other MQTT disconnected: %s", err.Error())
}

func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}
