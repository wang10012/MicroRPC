package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultPath    = "/micro-rpc/registry"
	defaultTimeout = time.Minute * 5
)

type ServerStatus struct {
	Address   string
	startTime time.Time
}

// Registry :register center
// receive heartbeat,make sure server alive
type Registry struct {
	timeout time.Duration            // alive timeout
	mu      sync.Mutex               // protect servers
	servers map[string]*ServerStatus // key: address
}

// addServer add a new server
// if a server has existed, refresh its startTime
func (r *Registry) addServer(address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[address]
	if s == nil {
		r.servers[address] = &ServerStatus{Address: address, startTime: time.Now()}
	} else {
		s.startTime = time.Now()
	}
}

// returnAliveServers: return alive servers addresses
// if a server timeout,delete it
func (r *Registry) returnAliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var aliveServers []string
	for address, server := range r.servers {
		if r.timeout == 0 || server.startTime.Add(r.timeout).After(time.Now()) {
			aliveServers = append(aliveServers, address)
		} else {
			delete(r.servers, address)
		}
	}
	// sort by key:address
	sort.Strings(aliveServers)
	return aliveServers
}

// Runs at /micro-rpc/registry
// A simple implementation,put server on req.Header
// GET: return all alive servers
// PUT: add new server or send heartbeat
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		// put server on req.Header
		// custom field name "micro-rpc-servers"
		w.Header().Set("micro-rpc-servers", strings.Join(r.returnAliveServers(), ","))
	case "POST":
		// custom field name "micro-rpc-server"
		address := req.Header.Get("micro-rpc-server")
		if address == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.addServer(address)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *Registry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func NewRegistry(timeout time.Duration) *Registry {
	return &Registry{
		timeout: timeout,
		servers: make(map[string]*ServerStatus),
	}
}

var DefaultRegistry = NewRegistry(defaultTimeout)

func HandleHTTP() {
	DefaultRegistry.HandleHTTP(defaultPath)
}

// HeartBeat send a heartbeat message every once in a while
func HeartBeat(serverAddr string, duration time.Duration, registryUrl string) {
	if duration == 0 {
		// 4 min
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartBeat(serverAddr, registryUrl)
	go func() {
		t := time.NewTicker(duration)
		// always send heartbeat until err occurs
		for err == nil {
			<-t.C
			err = sendHeartBeat(serverAddr, registryUrl)
		}
	}()
}

func sendHeartBeat(serverAddr string, registryUrl string) error {
	log.Println(serverAddr, "send heart beat to registry", registryUrl)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registryUrl, nil)
	req.Header.Set("micro-rpc-server", serverAddr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
