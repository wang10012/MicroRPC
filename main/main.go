package main

import (
	"MicroRPC"
	"fmt"
	"log"
	"net"
	"sync"
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

	client, _ := MicroRPC.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	// send request & receive response
	// Synchronization
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("micro rpc req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)
	}
	wg.Wait()
	//// mock easy client
	//conn, _ := net.Dial("tcp", <-addr)
	//defer func() { _ = conn.Close() }()
	//
	//time.Sleep(time.Second)
	//
	//// send options
	//// call Encode() == send
	//_ = json.NewEncoder(conn).Encode(MicroRPC.DefaultOption)
	//cp := encode.NewGobCodeProcess(conn)
	//// send request & receive response
	//for i := 0; i < 5; i++ {
	//	h := &encode.Header{
	//		ServiceMethod: "Foo.Sum",
	//		Seq:           uint64(i),
	//	}
	//	// call Write == send
	//	_ = cp.Write(h, fmt.Sprintf("micro rpc req %d", h.Seq))
	//	_ = cp.ReadHeader(h)
	//	var reply string
	//	_ = cp.ReadBody(&reply)
	//	log.Println("reply:", reply)
	//}
}
