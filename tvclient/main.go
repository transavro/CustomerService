package main

import (
	"fmt"
	"google.golang.org/grpc"
	"log"
	"os/exec"
)



func main() {

	conn, err := grpc.Dial("192.168.0.106:5000", grpc.WithInsecure())
	if err != nil {
		log.Fatalln("net.Dial:", err)
	}
	defer conn.Close()
	client := NewCustomerServiceClient(conn)

	var tvEmac string
	//ot , _ := exec.Command("sh", "-c", "cat /sys/class/net/eth0/address").Output()
	//tvEmac = string(ot)
	tvEmac = "70:2e:d9:55:44:33"
	sid, err := Authorize(client, tvEmac)
	if err != nil {
		log.Fatalln("authorize:", err)
	}
	log.Printf("TV == SessionID ==>%s", string(sid[:]))

	events, err := Connect(client, sid)
	if err != nil {
		log.Fatalln("connect:", err)
	}

	go func() {
		for {
			select {
			case event := <-events:
				switch {
				case event.GetJoin() != nil:
					fmt.Printf("%s has joined.\n", event.GetJoin().Name)
				case event.GetLeave() != nil:
					fmt.Printf("%s has left.\n", event.GetLeave().Name)
				case event.GetLog() != nil:
					{
						err := Say(client, tvEmac, event.GetLog().GetName(), HandleMessage(event.GetLog().Message))
						if err != nil {
							log.Fatalln("say:", err)
						}
					}
				}
			}
		}
	}()

	select {}
}

func HandleMessage(recivedMessage *Message) *Message {
	sendMessage := new(Message)
	sendMessage.MessageType = MessageType_REPONSE_MESSAGE
	if recivedMessage.GetMessageType() == MessageType_REQUEST_MESSAGE {
		log.Println("Executing command ===>  ",recivedMessage.GetCommand())
		ot, err := exec.Command("/system/bin/sh", "-c", recivedMessage.GetCommand()).Output()
		if err != nil {
			sendMessage.Command = fmt.Sprintf("error %s", err.Error())
		}
		sendMessage.Command = string(ot)
	}
	return sendMessage
}
