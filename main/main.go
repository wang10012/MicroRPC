package main

import (
	"MicroRPC"
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Wsj int

type Args struct{ Num1, Num2 int }

func (w *Wsj) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	// 3. (1) register Wsj.Sum
	var wsj Wsj
	// make sure pass a pointer
	if err := MicroRPC.Register(&wsj); err != nil {
		log.Fatal("register error:", err)
	}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	MicroRPC.HandleHTTP()
	addr <- l.Addr().String()
	// add http
	_ = http.Serve(l, nil)
	// without http
	//MicroRPC.Accept(l)
}

func main() {
	// log.SetFlags(0)
	// make sure start server,then client send message
	addr := make(chan string)
	go startServer(addr)

	client, _ := MicroRPC.DialHTTP("tcp", <-addr)
	defer func() { _ = client.Close() }()

	time.Sleep(time.Second)

	// 3. call method
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			// add call timeout control
			ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := client.Call(ctx, "Wsj.Sum", args, &reply); err != nil {
				log.Fatal("call Wsj.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()

	//// 2. send request & receive response,not call method
	//// Synchronization
	//var wg sync.WaitGroup
	//for i := 0; i < 5; i++ {
	//	wg.Add(1)
	//	go func(i int) {
	//		defer wg.Done()
	//		args := fmt.Sprintf("micro rpc req %d", i)
	//		var reply string
	//		if err := client.Call("Wsj.Sum", args, &reply); err != nil {
	//			log.Fatal("call Wsj.Sum error:", err)
	//		}
	//		log.Println("reply:", reply)
	//	}(i)
	//}
	//wg.Wait()

	//// 1. mock easy client
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
	//		ServiceMethod: "Wsj.Sum",
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
