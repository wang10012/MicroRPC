package loadbalance

import (
	. "MicroRPC"
	"context"
	"io"
	"reflect"
	"sync"
)

type BalanceClient struct {
	mode     ModeSelect
	discover Discover
	option   *Option
	clients  map[string]*Client // key:protocolAddr value:client For reusing the connections
	mu       sync.Mutex
}

func NewBalanceClient(mode ModeSelect, discover Discover, option *Option) *BalanceClient {
	return &BalanceClient{mode: mode, discover: discover, option: option, clients: make(map[string]*Client)}
}

func (bc *BalanceClient) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	for key, client := range bc.clients {
		_ = client.Close()
		delete(bc.clients, key)
	}
	return nil
}

var _ io.Closer = (*BalanceClient)(nil)

// add check if client can be reused and available
func (bc *BalanceClient) dial(protocolAddr string) (*Client, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	client, ok := bc.clients[protocolAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(bc.clients, protocolAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = GeneralDial(protocolAddr, bc.option)
		if err != nil {
			return nil, err
		}
		bc.clients[protocolAddr] = client
	}
	return client, nil
}

func (bc *BalanceClient) call(protocolAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := bc.dial(protocolAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

func (bc *BalanceClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	protocolAddr, err := bc.discover.Get(bc.mode)
	if err != nil {
		return err
	}
	return bc.call(protocolAddr, ctx, serviceMethod, args, reply)
}

// Broadcast call the named function for every server registered in discovery
// if there are errors ,return one of them
// if call successfully,return one res
func (bc *BalanceClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	services, err := bc.discover.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex // protect e and replyDone
	var e error
	replyDone := reply == nil // if reply is nil, don't need to set value
	ctx, cancel := context.WithCancel(ctx)
	for _, protocolAddr := range services {
		wg.Add(1)
		go func(protocolAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				// one of the calls success,creat a new clonedReply
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := bc.call(protocolAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // if any call failed, cancel unfinished calls
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(protocolAddr)
	}
	wg.Wait()
	return e
}
