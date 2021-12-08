package main

import (
	"MicroRPC"
	"MicroRPC/encode"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	MicroRPC.Accept(l)
}

func main() {
	log.SetFlags(0)
	// make sure start server,then client send message
	addr := make(chan string)
	go startServer(addr)

	// mock easy client
	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)

	// send options
	// call Encode() == send
	_ = json.NewEncoder(conn).Encode(MicroRPC.DefaultOption)
	cp := encode.NewGobCodeProcess(conn)
	// send request & receive response
	for i := 0; i < 5; i++ {
		h := &encode.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		// call Write == send
		_ = cp.Write(h, fmt.Sprintf("micro rpc req %d", h.Seq))
		_ = cp.ReadHeader(h)
		var reply string
		_ = cp.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}
