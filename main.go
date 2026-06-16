package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

const (
	DeviceID = "device-01"

	MQTTBroker = "ssl://f59ab996ae224d71a7a3187c60360d8d.s1.eu.hivemq.cloud:8883"
	MQTTUser   = "joaosvc"
	MQTTPass   = "qweqweROOT123"

	SerialPort = "/tmp/ttyAMA0"
)

var port serial.Port

var handleCommand mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	command := string(msg.Payload())

	if port != nil {
		switch command {
		case "CMD:PMP_1":
			port.Write([]byte("CMD:PUMP_ON\n"))

			fmt.Printf("Bomba ligada via MQTT\n")
		case "CMD:PMP_0":
			port.Write([]byte("CMD:PUMP_OFF\n"))
			fmt.Printf("Bomba desligada via MQTT\n")
		}
	}
}

func connectSerial() serial.Port {
	for {
		mode := &serial.Mode{
			BaudRate: 115200,
		}

		p, err := serial.Open(SerialPort, mode)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		return p
	}
}

func main() {
	opts := mqtt.NewClientOptions().AddBroker(MQTTBroker)
	opts.SetClientID(DeviceID)

	opts.SetUsername(MQTTUser)
	opts.SetPassword(MQTTPass)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
	}

	opts.SetTLSConfig(tlsConfig)

	opts.SetWill(fmt.Sprintf("dispositivos/%s/telemetria", DeviceID), "OFFLINE", 1, false)

	opts.SetDefaultPublishHandler(handleCommand)

	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Second)

	opts.OnConnect = func(c mqtt.Client) {
		fmt.Printf("Sistema conectado ao broker, ID: %s\n", DeviceID)
		c.Subscribe(fmt.Sprintf("dispositivos/%s/comando", DeviceID), 1, nil)
	}

	mqttClient := mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Erro ao conectar ao broker: %v", token.Error())
	}

	port = connectSerial()
	defer port.Close()

	fmt.Printf("Sistema iniciado com sucesso!\n")

	scanner := bufio.NewScanner(port)

	for scanner.Scan() {
		linha := scanner.Text()
		linha = strings.TrimSpace(linha)
		// fmt.Printf("Raw: %s\n", linha)

		if strings.Contains(linha, "D:") && strings.Contains(linha, ",R:") {
			mqttClient.Publish(fmt.Sprintf("dispositivos/%s/telemetria", DeviceID), 1, false, linha)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Erro ao ler dados da UART %s: %v", SerialPort, err)
	}
}
