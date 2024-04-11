package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
	"github.com/paulbellamy/ratecounter"
	"github.com/schollz/progressbar/v3"

	"github.com/google/uuid"
)

func main() {
	// Argument parsing
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")
	qos := flag.Int("q", 1, "MQTT QoS used by all clients")
	disableBar := flag.Bool("disable-bar", false, "Disable interactive mode to display statistics as log messages instead of interactive output")
	resetTime := flag.Float64("reset-after", 30, "Reset counter after <n> seconds without a message")
	cert := flag.String("cert", "", "Path to TLS certificate file")
	ca := flag.String("ca", "", "Path to TLS CA file")
	key := flag.String("key", "", "Path to TLS key file")
	insecure := flag.Bool("insecure", false, "Set to true to allow self signed certificates")
	mqtts := flag.Bool("mqtts", false, "Set to true to use MQTTS")
	cleanSession := flag.Bool("cleanSession", true, "Set to true for clean MQTT sessions or false to keep session")
	clientID := flag.String("clientID", "", "Custom MQTT clientID")
	keepAliveTimeout := flag.Int64("keepAliveTimeout", 5000, "Set the amount of time (in seconds) that the client should wait before sending a PING request to the broker")

	flag.Parse()

	if *qos < 0 || *qos > 2 {
		panic("QoS should be any of [0, 1, 2]")
	}

	if !*disableBar {
		fmt.Println("press ctrl+c to exit")
	}

	// General Client Config
	mqttClientConfig := MQTTClient.Config{
		TargetTopic:      targetTopic,
		Username:         username,
		Password:         password,
		Host:             host,
		Port:             port,
		QoS:              qos,
		Insecure:         insecure,
		MQTTS:            mqtts,
		CleanSession:     cleanSession,
		ClientID:         clientID,
		KeepAliveTimeout: keepAliveTimeout,
	}

	// If ca, cert, and key were set configure TLS
	if TLSOptionsSet() {
		mqttClientConfig.TLSConfigured = true
		mqttClientConfig.CA = ca
		mqttClientConfig.Cert = cert
		mqttClientConfig.Key = key
	}

	rand.Seed(time.Now().UnixNano())
	updates := make(chan int)

	mqttClient := MQTTClient.Client{
		ID:      uuid.NewString(),
		Config:  mqttClientConfig,
		Updates: updates,
	}

	mqttClient.Connect()

	mqttClient.Subscribe(*targetTopic)
	if !*disableBar {
		bar := progressbar.Default(-1)
		go func(updates chan int) {
			for update := range updates {
				bar.Add(update)
			}
		}(updates)

		// There's some issue with bar update when traffic is not constant
		// so this go routine updates the bar with 0 just to get the total numbers right
		ticker := time.NewTicker(1 * time.Second)
		go func() {
			for {
				// Block until the clock ticks
				<-ticker.C
				// Update bar with 0 to update total
				bar.Add(0)
			}
		}()
	} else {
		// Store total number of received messages since start or last reset
		msgCount := 0
		// Create a rate counter, that holds the number of messages per second
		rateCounter := ratecounter.NewRateCounter(1 * time.Second)
		tickTime := time.Now()
		go func(updates chan int) {
			for update := range updates {
				// Add the number of received msgs to the current total
				msgCount += update
				// Increase the rate counter by the number of received messages for the current tick
				rateCounter.Incr(int64(update))
				// Mark the last time we received a message
				tickTime = time.Now()
			}
		}(updates)

		uptimeTicker := time.NewTicker(1 * time.Second)

		for {
			select {
			case <-uptimeTicker.C:
				// Every second, as long as there are messages being received
				if msgCount > 0 {
					// output the total number of messages and the current rate
					fmt.Printf("Received %d messages so far while handling %d msg/sec\n", msgCount, rateCounter.Rate())
					if time.Since(tickTime).Seconds() > *resetTime {
						// last received message is too long ago, reset counter and stop log messages
						fmt.Printf("Did not receive a message for at least %d seconds. Resetting counter.\n", int(*resetTime))
						fmt.Println("Log will continue when new messages arrive.")
						msgCount = 0
					}
				}
			}
		}
	}
	select {}
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
