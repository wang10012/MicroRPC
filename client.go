package MicroRPC

import (
	"MicroRPC/encode"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
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
func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.GoCall(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
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

// Dial connects to an RPC server at the specified network address
func Dial(network, address string, options ...*Option) (client *Client, err error) {
	option, err := parseOptions(options...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, option)
}
