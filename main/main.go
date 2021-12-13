// add registry
package main

import (
	"MicroRPC"
	"MicroRPC/loadbalance"
	"MicroRPC/registry"
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Wsj int

type Args struct{ Num1, Num2 int }

func (w Wsj) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (w Wsj) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}

func startRegistry(wg *sync.WaitGroup) {
	l, _ := net.Listen("tcp", ":9999")
	registry.HandleHTTP()
	wg.Done()
	_ = http.Serve(l, nil)
}

func startServer(registryUrl string, wg *sync.WaitGroup) {
	var wsj Wsj
	l, _ := net.Listen("tcp", ":0")
	server := MicroRPC.NewServer()
	_ = server.Register(&wsj)
	registry.HeartBeat("tcp@"+l.Addr().String(), 0, registryUrl)
	wg.Done()
	server.Accept(l)
}

func logPrint(callType string, serviceMethod string, args *Args, bc *loadbalance.BalanceClient, ctx context.Context) {
	var reply int
	var err error
	switch callType {
	case "call":
		err = bc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = bc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", callType, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", callType, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(registryUrl string) {
	rd := loadbalance.NewRegistryDiscovery(registryUrl, 0)
	bc := loadbalance.NewBalanceClient(loadbalance.RandomSelect, rd, nil)
	defer func() { _ = bc.Close() }()
	// send request & receive response
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logPrint("call", "Wsj.Sum", &Args{Num1: i, Num2: i * i}, bc, context.Background())
		}(i)
	}
	wg.Wait()
}

func broadcast(registryUrl string) {
	rd := loadbalance.NewRegistryDiscovery(registryUrl, 0)
	bc := loadbalance.NewBalanceClient(loadbalance.RandomSelect, rd, nil)
	defer func() { _ = bc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logPrint("broadcast", "Wsj.Sum", &Args{Num1: i, Num2: i * i}, bc, context.Background())
			// expect 2 - 5 timeout
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			logPrint("broadcast", "Wsj.Sleep", &Args{Num1: i, Num2: i * i}, bc, ctx)
		}(i)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	registryUrl := "http://localhost:9999/micro-rpc/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()

	time.Sleep(time.Second)
	wg.Add(2)
	go startServer(registryUrl, &wg)
	go startServer(registryUrl, &wg)
	wg.Wait()

	time.Sleep(time.Second)
	call(registryUrl)
	broadcast(registryUrl)
}

//// add load-balance
//package main
//
//import (
//	"MicroRPC"
//	"MicroRPC/loadbalance"
//	"context"
//	"log"
//	"net"
//	"sync"
//	"time"
//)
//
//type Wsj int
//
//type Args struct{ Num1, Num2 int }
//
//func (w Wsj) Sum(args Args, reply *int) error {
//	*reply = args.Num1 + args.Num2
//	return nil
//}
//
//func (w Wsj) Sleep(args Args, reply *int) error {
//	time.Sleep(time.Second * time.Duration(args.Num1))
//	*reply = args.Num1 + args.Num2
//	return nil
//}
//
//func startServer(addrCh chan string) {
//	var wsj Wsj
//	l, err := net.Listen("tcp", ":0")
//	if err != nil {
//		log.Fatal("network error:", err)
//	}
//	log.Println("start rpc server on", l.Addr())
//	server := MicroRPC.NewServer()
//	_ = server.Register(&wsj)
//	addrCh <- l.Addr().String()
//	server.Accept(l)
//}
//
//func logPrint(callType string, serviceMethod string, args *Args, bc *loadbalance.BalanceClient, ctx context.Context) {
//	var reply int
//	var err error
//	switch callType {
//	case "call":
//		err = bc.Call(ctx, serviceMethod, args, &reply)
//	case "broadcast":
//		err = bc.Broadcast(ctx, serviceMethod, args, &reply)
//	}
//	if err != nil {
//		log.Printf("%s %s error: %v", callType, serviceMethod, err)
//	} else {
//		log.Printf("%s %s success: %d + %d = %d", callType, serviceMethod, args.Num1, args.Num2, reply)
//	}
//}
//
//func call(addr1, addr2 string) {
//	d := loadbalance.NewDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
//	bc := loadbalance.NewBalanceClient(loadbalance.RandomSelect, d, nil)
//	defer func() { _ = bc.Close() }()
//	// send request & receive response
//	var wg sync.WaitGroup
//	for i := 0; i < 5; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			logPrint("call", "Wsj.Sum", &Args{Num1: i, Num2: i * i}, bc, context.Background())
//		}(i)
//	}
//	wg.Wait()
//}
//
//func broadcast(addr1, addr2 string) {
//	d := loadbalance.NewDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
//	bc := loadbalance.NewBalanceClient(loadbalance.RandomSelect, d, nil)
//	defer func() { _ = bc.Close() }()
//	var wg sync.WaitGroup
//	for i := 0; i < 5; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			logPrint("broadcast", "Wsj.Sum", &Args{Num1: i, Num2: i * i}, bc, context.Background())
//			// expect 2 - 5 timeout
//			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
//			logPrint("broadcast", "Wsj.Sleep", &Args{Num1: i, Num2: i * i}, bc, ctx)
//		}(i)
//	}
//	wg.Wait()
//}
//
//func main() {
//	// log.SetFlags(0)
//	ch1 := make(chan string)
//	ch2 := make(chan string)
//	// start two servers
//	go startServer(ch1)
//	go startServer(ch2)
//
//	addr1 := <-ch1
//	addr2 := <-ch2
//
//	time.Sleep(time.Second)
//	call(addr1, addr2)
//	broadcast(addr1, addr2)
//}

//package main
//
//import (
//	"MicroRPC"
//	"context"
//	"log"
//	"net"
//	"net/http"
//	"sync"
//	"time"
//)
//
//type Wsj int
//
//type Args struct{ Num1, Num2 int }
//
//func (w *Wsj) Sum(args Args, reply *int) error {
//	*reply = args.Num1 + args.Num2
//	return nil
//}
//
//func startServer(addr chan string) {
//	// 3. (1) register Wsj.Sum
//	var wsj Wsj
//	// make sure pass a pointer
//	if err := MicroRPC.Register(&wsj); err != nil {
//		log.Fatal("register error:", err)
//	}
//	l, err := net.Listen("tcp", ":0")
//	if err != nil {
//		log.Fatal("network error:", err)
//	}
//	log.Println("start rpc server on", l.Addr())
//	MicroRPC.HandleHTTP()
//	addr <- l.Addr().String()
//	// add http
//	_ = http.Serve(l, nil)
//	// without http
//	//MicroRPC.Accept(l)
//}
//
//func main() {
//	// log.SetFlags(0)
//	// make sure start server,then client send message
//	addr := make(chan string)
//	go startServer(addr)
//
//	client, _ := MicroRPC.DialHTTP("tcp", <-addr)
//	defer func() { _ = client.Close() }()
//
//	time.Sleep(time.Second)
//
//	// 3. call method
//	var wg sync.WaitGroup
//	for i := 0; i < 5; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			args := &Args{Num1: i, Num2: i * i}
//			var reply int
//			// add call timeout control
//			ctx, _ := context.WithTimeout(context.Background(), time.Second)
//			if err := client.Call(ctx, "Wsj.Sum", args, &reply); err != nil {
//				log.Fatal("call Wsj.Sum error:", err)
//			}
//			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
//		}(i)
//	}
//	wg.Wait()
//
//	//// 2. send request & receive response,not call method
//	//// Synchronization
//	//var wg sync.WaitGroup
//	//for i := 0; i < 5; i++ {
//	//	wg.Add(1)
//	//	go func(i int) {
//	//		defer wg.Done()
//	//		args := fmt.Sprintf("micro rpc req %d", i)
//	//		var reply string
//	//		if err := client.Call("Wsj.Sum", args, &reply); err != nil {
//	//			log.Fatal("call Wsj.Sum error:", err)
//	//		}
//	//		log.Println("reply:", reply)
//	//	}(i)
//	//}
//	//wg.Wait()
//
//	//// 1. mock easy client
//	//conn, _ := net.Dial("tcp", <-addr)
//	//defer func() { _ = conn.Close() }()
//	//
//	//time.Sleep(time.Second)
//	//
//	//// send options
//	//// call Encode() == send
//	//_ = json.NewEncoder(conn).Encode(MicroRPC.DefaultOption)
//	//cp := encode.NewGobCodeProcess(conn)
//	//// send request & receive response
//	//for i := 0; i < 5; i++ {
//	//	h := &encode.Header{
//	//		ServiceMethod: "Wsj.Sum",
//	//		Seq:           uint64(i),
//	//	}
//	//	// call Write == send
//	//	_ = cp.Write(h, fmt.Sprintf("micro rpc req %d", h.Seq))
//	//	_ = cp.ReadHeader(h)
//	//	var reply string
//	//	_ = cp.ReadBody(&reply)
//	//	log.Println("reply:", reply)
//	//}
//}
