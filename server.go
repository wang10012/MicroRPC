package MicroRPC

import (
	"MicroRPC/encode"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const rpcNumber = 0x3bef5c

type Option struct {
	RPCNumber      int
	EncodingType   encode.Type
	ConnectTimeout time.Duration
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	RPCNumber:      rpcNumber,
	EncodingType:   encode.GobType,
	ConnectTimeout: time.Second * 10,
}

type Server struct {
	// locked
	services sync.Map // key:service.name value:service
}

func NewServer() *Server {
	return &Server{}
}

// DefaultServer start a default server to accept the lis
var DefaultServer = NewServer()

// Register publishes the receiver's methods in the DefaultServer.
func Register(instance interface{}) error {
	return DefaultServer.Register(instance)
}

// Register publishes in the server the set of methods of the
func (server *Server) Register(instance interface{}) error {
	s := newService(instance)
	if _, duplicated := server.services.LoadOrStore(s.name, s); duplicated {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) findService(serviceMethod string) (s *service, m *method, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	sec, ok := server.services.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	// assert
	s = sec.(*service)
	m = s.methods[methodName]
	if m == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

// Accept for every lis
func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ConnectServer(conn)
	}
}

func (server *Server) ConnectServer(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()
	var option Option
	if err := json.NewDecoder(conn).Decode(&option); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	if option.RPCNumber != rpcNumber {
		log.Printf("rpc server: invalid rpc number %x", option.RPCNumber)
		return
	}
	f := encode.NewCodeProcessMap[option.EncodingType]
	if f == nil {
		log.Printf("rpc server: invalid encoding type %s", option.EncodingType)
		return
	}
	server.serverProcess(f(conn), &option)
}

// request stores all information of a call
type request struct {
	header *encode.Header
	// err = client.Call("Arith.Multiply", args, &reply)
	_service     *service
	_method      *method
	argv, replyv reflect.Value
}

// invalidRequest :a placeholder for response argv when recoverable error occurs
// Then send this to client as reply
// reply:
// (1) recoverable error
// (2) return message after normal processing
var invalidRequest = struct{}{}

func (server *Server) serverProcess(cp encode.CodeProcess, opt *Option) {
	mu := new(sync.Mutex)     // send a complete response
	wg := new(sync.WaitGroup) // make sure all handleRequest done
	for {
		// readRequest
		req, err := server.readRequest(cp)
		if err != nil {
			// 1. irrecoverable error:break
			if req == nil {
				break
			}
			// 2. recoverable error:continue
			req.header.Error = err.Error()
			// send error Response
			server.sendResponse(cp, req.header, invalidRequest, mu)
			continue
		}
		wg.Add(1)
		// handleRequest
		// cover sendResponse
		go server.handleRequest(cp, req, mu, wg, opt.HandleTimeout)
	}
	wg.Wait()
	_ = cp.Close()
}

func (server *Server) readRequest(cp encode.CodeProcess) (*request, error) {
	header, err := server.readRequestHeader(cp)
	if err != nil {
		return nil, err
	}
	req := &request{header: header}

	req._service, req._method, err = server.findService(header.ServiceMethod)

	if err != nil {
		return req, nil
	}

	req.argv = req._method.newArgv()
	req.replyv = req._method.newReplyv()

	// ReadBody need pointer as parameter
	args := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		args = req.argv.Addr().Interface()
	}
	if err = cp.ReadBody(args); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}

	return req, nil
}

func (server *Server) readRequestHeader(cp encode.CodeProcess) (*encode.Header, error) {
	var header encode.Header
	if err := cp.ReadHeader(&header); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &header, nil
}

func (server *Server) sendResponse(cp encode.CodeProcess, header *encode.Header, body interface{}, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	if err := cp.Write(header, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cp encode.CodeProcess, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	// call method
	defer wg.Done()

	called := make(chan struct{})

	go func() {
		err := req._service.call(req._method, req.argv, req.replyv)
		called <- struct{}{}
		if err != nil {
			req.header.Error = err.Error()
			// send error response
			server.sendResponse(cp, req.header, invalidRequest, sending)
			return
		}
		// send value response
		server.sendResponse(cp, req.header, req.replyv.Interface(), sending)
	}()

	if timeout == 0 {
		<-called
		// log.Println("call success")
		return
	}

	select {
	case <-time.After(timeout):
		req.header.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		server.sendResponse(cp, req.header, invalidRequest, sending)
	case <-called:
		log.Println("call success!")
	}
	// Todo: 1. why sent is needed? 2. goroutine leak?
}

// add http
const (
	defaultRPCPath   = "/micro-rpc"
	defaultDebugPath = "/debug/rpc"
)

// ServeHTTP server implements an http.Handler that answers RPC requests.
// Hijack() : take over the connection.Use http for creating connection,and then use rpc
func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	// take over the conn
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacker ", req.RemoteAddr, ": ", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+"200 connected to micro rpc"+"\n\n")
	log.Printf("HTTP/1.0 " + "200 connected to micro rpc" + "\n\n")
	server.ConnectServer(conn)
}

// HandleHTTP registers an HTTP handler for RPC messages on rpcPath.
func (server *Server) HandleHTTP() {
	http.Handle(defaultRPCPath, server)
}

// HandleHTTP default server register HTTP handlers
func HandleHTTP() {
	DefaultServer.HandleHTTP()
}
