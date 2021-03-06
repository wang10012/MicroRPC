package MicroRPC

import (
	"MicroRPC/encode"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Done          chan *Call
	Error         error
}

func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	seq      uint64
	cp       encode.CodeProcess
	option   *Option
	header   encode.Header
	sendLock sync.Mutex // protect send
	mu       sync.Mutex // Client may be used by multiple goroutines
	calling  map[uint64]*Call
	closing  bool
	shutdown bool
}

var ErrorShutdown = errors.New("connection is shut down")

// Close the connection
// implement interface Closer
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrorShutdown
	}
	client.closing = true
	return client.cp.Close()
}

var _ io.Closer = (*Client)(nil)

func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrorShutdown
	}
	call.Seq = client.seq
	client.calling[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.calling[seq]
	delete(client.calling, seq)
	return call
}

func (client *Client) terminateCalls(err error) {
	client.sendLock.Lock()
	defer client.sendLock.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.calling {
		call.Error = err
		// notify all calls
		call.done()
	}
}

// IsAvailable return true if the client is available
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}

func (client *Client) receive() {
	var err error
	// if err,break
	for err == nil {
		var h encode.Header
		if err = client.cp.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			err = client.cp.ReadBody(nil)
		// call exist,but server errors
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cp.ReadBody(nil)
			call.done()
		default:
			err = client.cp.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// terminateCalls when error
	client.terminateCalls(err)
}

func (client *Client) send(call *Call) {
	// make sure that the client will send a complete request
	client.sendLock.Lock()
	defer client.sendLock.Unlock()
	// register this call.
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		// notify err
		call.done()
		return
	}
	// prepare request header
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""
	// encode and send the request
	if err := client.cp.Write(&client.header, call.Args); err != nil {
		// err
		call := client.removeCall(seq)
		// call may be nil: Write failed,
		// call not nil: client has received the response and handled
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// GoCall invokes the function asynchronously.
func (client *Client) GoCall(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// Call invokes the named function, waits for it to complete,
func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := client.GoCall(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	// ctx, _ := context.WithTimeout(context.Background(), time.Second)
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("rpc client: call failed: " + ctx.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}

// NewHTTPClient new a Client instance via HTTP as transport protocol
func NewHTTPClient(conn net.Conn, opt *Option) (*Client, error) {
	_, _ = io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))
	log.Printf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath)

	// Require successful HTTP response before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == "200 connected to micro rpc" {
		return NewClient(conn, opt)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	return nil, err
}

func NewClient(connect net.Conn, option *Option) (*Client, error) {
	f := encode.NewCodeProcessMap[option.EncodingType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", option.EncodingType)
		log.Println("rpc client: encode error:", err)
		return nil, err
	}
	// send options to server
	if err := json.NewEncoder(connect).Encode(option); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = connect.Close()
		return nil, err
	}
	client := &Client{
		seq:     1, // 0: invalid call
		cp:      f(connect),
		option:  option,
		calling: make(map[uint64]*Call),
	}
	go client.receive()
	return client, nil
}

func parseOptions(options ...*Option) (*Option, error) {
	// options is nil
	// pass nil as parameter
	if len(options) == 0 || options[0] == nil {
		return DefaultOption, nil
	}
	if len(options) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	option := options[0]
	option.RPCNumber = DefaultOption.RPCNumber
	if option.EncodingType == "" {
		option.EncodingType = DefaultOption.EncodingType
	}
	return option, nil
}

// Dial :no ConnectTimeout Control
//// Dial connects to an RPC server at the specified network address
//func Dial(network, address string, options ...*Option) (client *Client, err error) {
//	option, err := parseOptions(options...)
//	if err != nil {
//		return nil, err
//	}
//	conn, err := net.Dial(network, address)
//	if err != nil {
//		return nil, err
//	}
//	// close the connection if client is nil
//	defer func() {
//		if client == nil {
//			_ = conn.Close()
//		}
//	}()
//	return NewClient(conn, option)
//}

// clientStatus: show client status
type clientStatus struct {
	client *Client
	err    error
}

//newClientFunc implement different dial by different ClientFunc
type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		// 1. after a duration,err == nil,means conn failed,so close the connection
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientStatus)
	go func() {
		client, err := f(conn, opt)
		ch <- clientStatus{client: client, err: err}
	}()
	// edge case of the value of ConnectTimeout
	// if ConnectTimeout == 0 then return res directly
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	// 2. NewClient() includes encode(write options to server) and go receive, so it may timeout
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// DialHTTP connects to an HTTP RPC server at the specified network address
// listening on the default HTTP RPC path.
func DialHTTP(network, address string, opts ...*Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, address, opts...)
}

func Dial(network, address string, options ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, options...)
}

// GeneralDial :protocolAddr is a general format (protocol@addr) to represent a rpc server
// eg, http@10.0.0.1:7001, tcp@10.0.0.1:9999
func GeneralDial(protocolAddr string, opts ...*Option) (*Client, error) {
	parts := strings.Split(protocolAddr, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rpc client err: wrong format '%s', expect protocol@addr", protocolAddr)
	}
	protocol, addr := parts[0], parts[1]
	switch protocol {
	case "http":
		return DialHTTP("tcp", addr, opts...)
	default:
		// tcp, unix or other transport protocol
		return Dial(protocol, addr, opts...)
	}
}
