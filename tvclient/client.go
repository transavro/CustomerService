package main

import (
	"golang.org/x/net/context"
	"io"
	"log"
)

func Authorize(client CustomerServiceClient, name string) (sid []byte, err error) {
	req := RequestAuthorize{
		Name: name,
	}
	res, err := client.Authorize(context.Background(), &req)
	if err != nil {
		return
	}
	sid = res.SessionId
	return
}

func Connect(client CustomerServiceClient, sid []byte) (events chan *Event, err error) {
	req := RequestConnect{
		SessionId: sid,
	}
	stream, err := client.Connect(context.Background(), &req)
	if err != nil {
		return
	}
	events = make(chan *Event, 1000)
	go func() {
		defer func() { close(events) }()
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Fatalln("stream.Recv", err)
			}
			events <- event
		}
	}()
	return
}

func Say(client CustomerServiceClient, sid string, tsid string,  message *Message) error {
	req := CommandSay{
		SourceName: sid,
		Message:         message,
		TargetName: tsid,
	}
	_, err := client.Say(context.Background(), &req)
	return err
}
