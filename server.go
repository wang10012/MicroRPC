package MicroRPC

import (
	"MicroRPC/encode"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const rpcNumber = 0x3bef5c

type Option struct {
	rpcNumber    int
	EncodingType encode.Type
}

var DefaultOption = &Option{
	rpcNumber: rpcNumber,
	EncodingType: encode.GobType,
}

type Server struct{}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

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

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
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
	if option.rpcNumber != rpcNumber {
		log.Printf("rpc server: invalid rpc number %x", option.rpcNumber)
		return
	}
	f := encode.NewCodeProcessMap[option.EncodingType]
	if f == nil {
		log.Printf("rpc server: invalid encoding type %s", option.EncodingType)
		return
	}
	server.serverProcess(f(conn))
}

var invalidRequest = struct{}{}

func (server *Server) serverProcess(cp encode.CodeProcess) {
	mu := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		// readRequest
		req, err := server.readRequest(cp)
		if err != nil {
			if req == nil {
				break
			}
			req.header.Error = err.Error()
			// send error Response
			server.sendResponse(cp, req.header, invalidRequest, mu)
			continue
		}
		wg.Add(1)
		// handleRequest
		// cover sendResponse
		go server.handleRequest(cp, req, mu, wg)
	}
	wg.Wait()
	_ = cp.Close()
}

// request stores all information of a call
type request struct {
	header       *encode.Header
	// err = client.Call("Arith.Multiply", args, &reply)
	// ToDo ???
	argv, replyv reflect.Value
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

func (server *Server) readRequest(cp encode.CodeProcess) (*request, error) {
	header, err := server.readRequestHeader(cp)
	if err != nil {
		return nil, err
	}
	req := &request{header: header}
	// TODO: now we don't know the type of request argv
	// day 1, just suppose it's string
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = cp.ReadBody(req.argv.Interface()); err != nil {
		log.Println("rpc server: read argv err:", err)
	}
	return req, nil
}

func (server *Server) sendResponse(cp encode.CodeProcess, header *encode.Header, body interface{}, mu *sync.Mutex) {
	mu.Lock()
	defer mu.Unlock()
	if err := cp.Write(header, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cp encode.CodeProcess, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	// TODO, should call registered rpc methods to get the right replyv
	// day 1, just print argv and send a hello message
	defer wg.Done()
	log.Println(req.header, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.header.Seq))
	server.sendResponse(cp, req.header, req.replyv.Interface(), sending)
}
