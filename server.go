package main

import (
	"errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

const sessionIDLength = 16

type sessionId [sessionIDLength]byte

type chatServer struct {
	mu   sync.RWMutex
	name map[sessionId]string
	buf  map[sessionId]chan *Event
	last time.Time
	in   int64
	out  int64
}

func newChatServer() *chatServer {
	return &chatServer{
		name: make(map[sessionId]string),
		buf:  make(map[sessionId]chan *Event),
		last: time.Now(),
	}
}

func (cs *chatServer) GenerateSessionId() sessionId {
	var sid sessionId
	for i := 0; i < sessionIDLength/4; i++ {
		r := rand.Uint32()
		for j := 0; j < 4 && i*4+j < sessionIDLength; j++ {
			sid[i*4+j] = byte(r)
			r >>= 8
		}
	}
	return sid
}

func (cs *chatServer) withWriteLock(f func()) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	f()
}

func (cs *chatServer) withReadLock(f func()) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	f()
}

func (cs *chatServer) unsafeExpire(id sessionId) {
	if buf, ok := cs.buf[id]; ok {
		close(buf)
	}
	delete(cs.buf, id)
	delete(cs.name, id)
}

func (cs *chatServer) alignSessionID(raw []byte) sessionId {
	var sid sessionId
	for i := 0; i < sessionIDLength && i < len(raw); i++ {
		sid[i] = raw[i]
	}
	return sid
}

func (cs *chatServer) Authorize(ctx context.Context, req *RequestAuthorize) (*ResponseAuthorize, error) {
	cs.in++
	if len(req.Name) == 0 {
		return nil, errors.New("name must be not empty")
	}
	if len(req.Name) > 20 {
		return nil, errors.New("name must be less than or equal 20 characters")
	}
	sid := cs.GenerateSessionId()
	cs.withWriteLock(func() {
		cs.name[sid] = req.GetName()
	})
	go func() {
		time.Sleep(5 * time.Second)
		cs.withWriteLock(func() {
			if _, ok := cs.buf[sid]; ok {
				return
			}
			cs.unsafeExpire(sid)
		})
	}()

	res := ResponseAuthorize{
		SessionId: sid[:],
	}
	return &res, nil
}

func (cs *chatServer) Connect(req *RequestConnect, stream CustomerService_ConnectServer) error {
	cs.in++
	var (
		sid  = cs.alignSessionID(req.GetSessionId())
		buf  = make(chan *Event, 1000)
		name string
		err  error
	)
	cs.withWriteLock(func() {
		var ok bool
		name, ok = cs.name[sid]
		if !ok {
			err = errors.New("not an authorized User")
			return
		}
		if _, ok := cs.buf[sid]; ok {
			err = errors.New("already connected")
			return
		}
		cs.buf[sid] = buf
	})
	if err != nil {
		return err
	}

	go cs.withReadLock(func() {
		log.Printf("Joining name ==> %s", name)
		for _, buf := range cs.buf {
			buf <- &Event{Status: &Event_Join{Join: &EventJoin{Name: name}}}
		}
	})

	defer cs.withReadLock(func() {
		log.Printf("Leave name=%s", name)
		for _, buf := range cs.buf {
			buf <- &Event{Status: &Event_Leave{Leave: &EventLeave{Name: name}}}
		}
	})

	defer cs.withWriteLock(func() { cs.unsafeExpire(sid) })

	tick := time.Tick(time.Second)
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case event := <-buf:
			if err = stream.Send(event); err != nil {
				return err
			}
			cs.out++
		case <-tick:
			if err := stream.Send(&Event{Status: &Event_None{None: &EventNone{}}}); err != nil {
				return err
			}
			cs.out++
		}
	}
}

func (cs *chatServer) Say(ctx context.Context, req *CommandSay) (*None, error) {
	cs.in++
	log.Println(req.GetMessage())

	var (
		sid  = cs.nameToID(req.GetSourceName())
		name string
		err  error
	)
	cs.withReadLock(func() {
		var ok bool
		name, ok = cs.name[sid]
		if !ok {
			err = errors.New("not authorized")
			return
		}
		if _, ok := cs.buf[sid]; !ok {
			err = errors.New("not authorized")
			return
		}
	})
	if err != nil {
		return nil, err
	}

	go cs.withReadLock(func() {
		log.Printf("Log name=%s message=%s", name, req.Message)

		tSid := cs.nameToID(req.GetTargetName())
		cs.buf[tSid] <- &Event{Status: &Event_Log{Log: &EventLog{
			Name:    name,
			Message: req.GetMessage(),
		}}}
	})
	return &None{}, nil
}

func(cs *chatServer) nameToID(name string) sessionId {
	for id, s := range cs.name {
		if name == s {
			return id
		}
	}
	return [16]byte{}
}

func main() {
	//Parsing the cmd flags
	lis, err := net.Listen("tcp", ":5000")
	if err != nil {
		panic("error while init server")
	}
	cs := newChatServer()
	go func() {
		tick := time.Tick(time.Second)
		for {
			select {
			case <-tick:
				now := time.Now()
				duration := time.Now().Sub(cs.last)
				log.Printf("IN=%d OUT=%d IPS=%.0f OPS=%.0f auth=%d connect=%d",
					cs.in,
					cs.out,
					float64(cs.in)/float64(duration)*float64(time.Second),
					float64(cs.out)/float64(duration)*float64(time.Second),
					len(cs.name),
					len(cs.buf),
				)
				cs.last = now
				cs.in = 0
				cs.out = 0
			}
		}
	}()
	server := grpc.NewServer()
	RegisterCustomerServiceServer(server, cs)
	if err := server.Serve(lis); err != nil {
		log.Fatalln("Serve:", err)
	}
}
