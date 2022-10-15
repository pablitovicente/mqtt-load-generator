package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	MQTTClient "github.com/pablitovicente/mqtt-load-generator/pkg/MQTTClient"
	"github.com/schollz/progressbar/v3"
)

func main() {
	// Argument parsing
	targetTopic := flag.String("t", "/load", "Target MQTT topic to publish messages to")
	username := flag.String("u", "", "MQTT username")
	password := flag.String("P", "", "MQTT password")
	host := flag.String("h", "localhost", "MQTT host")
	port := flag.Int("p", 1883, "MQTT port")

	flag.Parse()

	fmt.Println("press ctrl+c to exit")

	// General Client Config
	mqttClientConfig := MQTTClient.Config{
		TargetTopic: targetTopic,
		Username:    username,
		Password:    password,
		Host:        host,
		Port:        port,
	}

	rand.Seed(time.Now().UnixNano())
	updates := make(chan int)

	mqttClient := MQTTClient.Client{
		ID:     rand.Intn(100000),
		Config: mqttClientConfig,
		Updates: updates,
	}

	mqttClient.Connect()

	mqttClient.Subscribe(*targetTopic)
	bar := progressbar.Default(-1)
	go func(updates chan int) {
		for update := range updates {
			bar.Add(update)
		}
	}(updates)

	select {}
}
