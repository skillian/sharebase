package web

import (
	"sync"
)

// ClientPool holds a pool of ShareBase clients.  Clients for multiple data
// centers with multiple authentication tokens can be stored in the same
// pool.  ClientPools have a mutex to make accessing and caching clients safe
// but clients themselves cannot be used concurrently.
type ClientPool struct {
	mutex    sync.Mutex
	subPools map[clientPoolKey]*clientSubPool
}

// NewClientPool creates a new pool of Clients.
func NewClientPool() *ClientPool {
	return &ClientPool{
		mutex:    sync.Mutex{},
		subPools: make(map[clientPoolKey]*clientSubPool),
	}
}

// Client gets an existing cached client or creates one.
func (p *ClientPool) Client(dataCenter, token string) (*Client, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if c, ok := p.getOrCreateSubPool(clientPoolKey{dataCenter, token}).getClient(); ok {
		return c, nil
	}
	return NewClient(dataCenter, token)
}

// Cache the given client in the pool.  It is not necessary for the client to
// have been created from the pool.  It is critical that the client not be
// used after it has been returned to the client pool.  If a client is in any
// broken state, it should not be returned to the pool; simply request a new
// client.
func (p *ClientPool) Cache(c *Client) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	key := clientPoolKey{
		dataCenter: c.DataCenter.String(),
		token:      c.phoenixToken[len(PhoenixTokenPrefix)+1:],
	}
	p.getOrCreateSubPool(key).cacheClient(c)
}

func (p *ClientPool) getOrCreateSubPool(k clientPoolKey) *clientSubPool {
	sp, ok := p.subPools[k]
	if !ok {
		sp = newClientSubPool()
		p.subPools[k] = sp
	}
	return sp
}

type clientPoolKey struct {
	dataCenter string
	token      string
}

type clientSubPool struct {
	clients []*Client
	cache   []int
	inuse   map[*Client]int
}

func newClientSubPool() *clientSubPool {
	return &clientSubPool{
		clients: make([]*Client, 0, 1),
		cache:   make([]int, 0, 1),
		inuse:   make(map[*Client]int, 1),
	}
}

// getClient pulls a cached client from the subpool.  This function is not
// threadsafe; make sure you only use it while holding the ClientPool's lock.
func (sp *clientSubPool) getClient() (*Client, bool) {
	length := len(sp.cache)
	if length == 0 {
		return nil, false
	}
	index := sp.cache[length-1]
	sp.cache = sp.cache[:length-1]
	client := sp.clients[index]
	sp.inuse[client] = index
	return client, true
}

// cacheClient stores a client into the pool of clients.
func (sp *clientSubPool) cacheClient(c *Client) {
	index, ok := sp.inuse[c]
	if !ok {
		index = len(sp.clients)
		sp.clients = append(sp.clients, c)
	}
	delete(sp.inuse, c)
	sp.cache = append(sp.cache, index)
}
