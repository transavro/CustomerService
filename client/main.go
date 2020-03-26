package main

import (
	"bufio"
	"fmt"
	"google.golang.org/grpc"
	"io"
	"log"
	"os"
	"strings"
)

func main() {

	conn, err := grpc.Dial(":5000", grpc.WithInsecure())
	if err != nil {
		log.Fatalln("net.Dial:", err)
	}
	defer conn.Close()
	client := NewCustomerServiceClient(conn)

	var name string
	for {
		fmt.Print("name> ")
		if n, err := fmt.Scanln(&name); err == io.EOF {
			return
		} else if n == 0 {
			fmt.Println("name must be not empty")
			continue
		} else if n > 20 {
			fmt.Println("name must be less than or equal 20 characters")
			continue
		}
		break
	}

	sid, err := Authorize(client, name)
	if err != nil {
		log.Fatalln("authorize:", err)
	}

	log.Printf("%s == SessionID ==>%s", name, fmt.Sprintln(string(sid)))

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
					fmt.Printf("%s> %s\n", event.GetLog().Name, event.GetLog().Message)
				}
			}
		}
	}()

	var message string
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Chat Shell")
	fmt.Println("---------------------")
	for {
		fmt.Print("-> ")
		message, _ = reader.ReadString('\n')
		// convert CRLF to LF
		message = strings.Replace(message, "\n", "", -1)
		log.Printf("===>%s<===", message)
		serverMsg := new(Message)
		serverMsg.MessageType = MessageType_REQUEST_MESSAGE
		serverMsg.Command = message
		err := Say(client, name, "70:2e:d9:55:44:33", serverMsg)
		if err != nil {
			log.Fatalln("say:", err)
		}
	}
}
