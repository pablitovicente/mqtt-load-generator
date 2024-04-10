package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
	"github.com/schollz/progressbar/v3"
)

func main() {
	// Argument parsing
	messageCount := flag.Int("c", 1000, "Number of messages to send")
	messageSize := flag.Int("s", 100, "Size in bytes of the message payload")
	interval := flag.Int("i", 1, "Milliseconds to wait between messages")
	schedule := flag.String("z", "normal", "Distribution of time between messages: 'flat': always wait Interval between messages, 'normal': wait a random amount between messages with mean equal to the interval and stdev to half interval, 'random': wait a random amount between messages with mean equal to the interval.")
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")
	numberOfClients := flag.Int("n", 1, "Number of concurrent MQTT clients")
	idAsSubTopic := flag.Bool("suffix", false, "If set to true integers will be used as sub-topic to the topic specified by 't'. The range goes from 1 to N where N is the max number of configured concurrent clients.")
	qos := flag.Int("q", 1, "MQTT QoS used by all clients")
	cert := flag.String("cert", "", "Path to TLS certificate file")
	ca := flag.String("ca", "", "Path to TLS CA file")
	key := flag.String("key", "", "Path to TLS key file")
	insecure := flag.Bool("insecure", false, "Set to true to allow self signed certificates")
	mqtts := flag.Bool("mqtts", false, "Set to true to use MQTTS")
	cleanSession := flag.Bool("cleanSession", true, "Set to true for clean MQTT sessions or false to keep session")
	clientID := flag.String("clientID", "", "Custom MQTT clientID")

	flag.Parse()

	if *qos < 0 || *qos > 2 {
		panic("QoS should be any of [0, 1, 2]")
	}

	// General Client Config
	mqttClientConfig := MQTTClient.Config{
		MessageCount: messageCount,
		MessageSize:  messageSize,
		Interval:     interval,
		Schedule:     schedule,
		TargetTopic:  targetTopic,
		Username:     username,
		Password:     password,
		Host:         host,
		Port:         port,
		IdAsSubTopic: idAsSubTopic,
		QoS:          qos,
		Insecure:     insecure,
		MQTTS:        mqtts,
		CleanSession: cleanSession,
		ClientID:     clientID,
	}
	// If ca, cert, and key were set configure TLS
	if TLSOptionsSet() {
		mqttClientConfig.TLSConfigured = true
		mqttClientConfig.CA = ca
		mqttClientConfig.Cert = cert
		mqttClientConfig.Key = key
	}

	updates := make(chan int)
	connectionProgress := make(chan int)

	pool := MQTTClient.Pool{
		SetupDone:   make(chan struct{}),
		MqttClients: make([]*MQTTClient.Client, 0),
	}

	// Provide feedback about connection progress
	connectionBar := createConnectionsBar(int64(*numberOfClients))

	go func(progress chan int) {
		for range progress {
			connectionBar.Add(1)
			// Stop connection progress
			if connectionBar.IsFinished() {
				connectionBar.Exit()
				connectionBar.Close()
			}
		}
	}(connectionProgress)

	pool.New(numberOfClients, mqttClientConfig, updates, connectionProgress)
	// Wait until all the setup is done
	<-pool.SetupDone

	var wg sync.WaitGroup
	pool.Start(&wg)

	bar := createMessageProgressBar(int64(*numberOfClients), int64(*messageCount))
	go func(updates chan int) {
		for update := range updates {
			bar.Add(update)
		}
	}(updates)

	wg.Wait()
	bar.Close()
}

func TLSOptionsSet() bool {
	foundCert := false
	foundCA := false
	foundKey := false

	flag.Visit(func(f *flag.Flag) {
		if f.Name == "cert" {
			foundCert = true
		}

		if f.Name == "ca" {
			foundCA = true
		}

		if f.Name == "key" {
			foundKey = true
		}
	})

	return foundCA && foundCert && foundKey
}

func createConnectionsBar(numberOfClients int64) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		int64(numberOfClients),
		progressbar.OptionSetDescription(fmt.Sprintf("Connecting %d MQTT clients", numberOfClients)),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)
}

func createMessageProgressBar(numberOfClients int64, messageCount int64) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		numberOfClients*messageCount,
		progressbar.OptionSetDescription(fmt.Sprintf("Publishing %d messages", numberOfClients*messageCount)),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowElapsedTimeOnFinish(),
	)
}
