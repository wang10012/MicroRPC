package loadbalance

import (
	"log"
	"net/http"
	"strings"
	"time"
)

const defaultUpdateTimeout = time.Second * 10

type RegistryDiscovery struct {
	*Discovery
	registryUrl    string
	updateTimeOut  time.Duration // server lists timeout
	lastUpdateTime time.Time
}

func (rd *RegistryDiscovery) Update(services []string) error {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.services = services
	rd.lastUpdateTime = time.Now()
	return nil
}

func (rd *RegistryDiscovery) Refresh() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	// Determine if it is expired
	// needn't to refresh
	if rd.lastUpdateTime.Add(rd.updateTimeOut).After(time.Now()) {
		return nil
	}
	log.Println("rpc registry: refresh servers from registry", rd.registryUrl)
	resp, err := http.Get(rd.registryUrl)
	if err != nil {
		log.Println("rpc registry refresh err:", err)
		return err
	}
	servers := strings.Split(resp.Header.Get("micro-rpc-servers"), ",")
	rd.services = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			rd.services = append(rd.services, strings.TrimSpace(server))
		}
	}
	rd.lastUpdateTime = time.Now()
	return nil
}

func (rd *RegistryDiscovery) Get(mode ModeSelect) (string, error) {
	if err := rd.Refresh(); err != nil {
		return "", err
	}
	return rd.Discovery.Get(mode)
}

func (rd *RegistryDiscovery) GetAll() ([]string, error) {
	if err := rd.Refresh(); err != nil {
		return nil, err
	}
	return rd.Discovery.GetAll()
}

func NewRegistryDiscovery(registryUrl string, updateTimeOut time.Duration) *RegistryDiscovery {
	if updateTimeOut == 0 {
		updateTimeOut = defaultUpdateTimeout
	}
	rd := &RegistryDiscovery{
		Discovery:     NewDiscovery(make([]string, 0)),
		registryUrl:   registryUrl,
		updateTimeOut: updateTimeOut,
	}
	return rd
}
