package MQTTClient

type Pool struct {
	MqttClients []Client
	SetupDone chan struct{}
}

func (p *Pool) New(numOfClients *int, clientConfig Config, updates chan int) {
	// Configure the required number of clients
	for c := 1; c <= *numOfClients; c++ {
		mqttClient := Client{
			ID:     c,
			Config: clientConfig,
			Updates: updates,
		}
		// Connect
		mqttClient.Connect()
		p.MqttClients = append(p.MqttClients, mqttClient)
	}

	close(p.SetupDone)
}

func (p *Pool) Start() {
	for _, c := range p.MqttClients {
		go c.Start()
	}
}