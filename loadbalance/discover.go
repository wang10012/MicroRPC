package loadbalance

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type ModeSelect int

const (
	RandomSelect     ModeSelect = iota // select randomly
	RoundRobinSelect                   // select using Robbin algorithm
)

type Discover interface {
	Refresh() error // refresh from remote registry center
	Update(services []string) error
	Get(mode ModeSelect) (string, error)
	GetAll() ([]string, error)
}

// Discovery :without a registry center ,user provides the server addresses explicitly instead
type Discovery struct {
	seed     *rand.Rand // random seed
	index    int        // record the selected position for robin algorithm
	mu       sync.RWMutex
	services []string // protocolAddr of every server
}

func NewDiscovery(services []string) *Discovery {
	d := &Discovery{
		seed:     rand.New(rand.NewSource(time.Now().UnixNano())),
		services: services,
	}
	d.index = d.seed.Intn(math.MaxInt32 - 1)
	return d
}

// Refresh
// Now return nil when without a registry center
func (d *Discovery) Refresh() error {
	return nil
}

// Update the services of discovery dynamically if needed
func (d *Discovery) Update(services []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.services = services
	return nil
}

// Get a server according to mode
func (d *Discovery) Get(mode ModeSelect) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(d.services)
	if n == 0 {
		return "", errors.New("rpc discovery: no available services")
	}
	switch mode {
	case RandomSelect:
		return d.services[d.seed.Intn(n)], nil
	case RoundRobinSelect:
		s := d.services[d.index%n]
		d.index = (d.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: not supported any select mode")
	}
}

// GetAll returns all services in discovery
func (d *Discovery) GetAll() ([]string, error) {
	// Todo: RwLock ?
	d.mu.RLock()
	defer d.mu.RUnlock()
	// return a copy of d.services
	services := make([]string, len(d.services), len(d.services))
	copy(services, d.services)
	return services, nil
}

var _ Discover = (*Discovery)(nil)
