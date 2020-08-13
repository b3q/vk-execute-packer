package packer

import (
	"log"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/api"
)

// FilterMode ...
type FilterMode bool

const (
	// Allow mode
	Allow FilterMode = true
	// Ignore mode
	Ignore FilterMode = false
)

// Packer struct
type Packer struct {
	lastFlushTime     time.Time
	maxPackedRequests int
	tokenPool         *tokenPool
	tokenLazyLoading  bool
	filterMode        FilterMode
	filterMethods     map[string]struct{}
	mtx               sync.RWMutex
	debug             bool
	handler           func(string, api.Params) (api.Response, error)
	currentBatch      *batch
}

// Option func
type Option func(*Packer)

// MaxPackedRequests opt
func MaxPackedRequests(max int) Option {
	if max < 1 || max > 25 {
		max = 25
	}
	return func(p *Packer) {
		p.maxPackedRequests = max
	}
}

// Rules opt
func Rules(mode FilterMode, methods ...string) Option {
	return func(p *Packer) {
		for _, m := range methods {
			p.filterMode = mode
			p.filterMethods[m] = struct{}{}
		}
	}
}

// Debug opt
func Debug() Option {
	return func(p *Packer) {
		p.debug = true
	}
}

// Tokens opt
func Tokens(tokens ...string) Option {
	return func(p *Packer) {
		p.tokenLazyLoading = false
		p.tokenPool = newTokenPool(tokens...)
	}
}

// New ...
func New(vk *api.VK, opts ...Option) *Packer {
	p := &Packer{
		tokenLazyLoading:  true,
		tokenPool:         newTokenPool(),
		lastFlushTime:     time.Now(),
		maxPackedRequests: 25,
		filterMode:        Ignore,
		filterMethods:     make(map[string]struct{}),
		handler:           vk.Handler,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.currentBatch = newBatch(p.execute, p.debug)

	return p
}

// Default func
func Default(vk *api.VK, opts ...Option) {
	p := New(vk, opts...)
	go TimeoutTrigger(time.Second, p)
	vk.Handler = p.Handler
}

// Handler func
func (p *Packer) Handler(method string, params api.Params) (api.Response, error) {
	if p.debug {
		log.Printf("packer: Handler call (%s)\n", method)
	}

	if method == "execute" {
		return p.handler(method, params)
	}

	_, found := p.filterMethods[method]
	if (p.filterMode == Allow && !found) ||
		(p.filterMode == Ignore && found) {
		return p.handler(method, params)
	}

	if p.tokenLazyLoading {
		tokenIface, ok := params["access_token"]
		if !ok {
			panic("packer: missing access_token param")
		}

		token, ok := tokenIface.(string)
		if !ok {
			panic("packer: bad access_token type")
		}

		p.tokenPool.append(token)
	}

	p.mtx.RLock()
	var (
		resp api.Response
		err  error
		wg   sync.WaitGroup
	)
	wg.Add(1)
	handler := func(r api.Response, e error) {
		resp = r
		err = e
		wg.Done()
	}

	p.currentBatch.Request(method, params, handler)
	needFlush := p.currentBatch.Count() == p.maxPackedRequests
	p.mtx.RUnlock()

	if needFlush {
		p.Flush()
	}

	wg.Wait()
	return resp, err
}

// Flush func
func (p *Packer) Flush() {
	if p.debug {
		log.Println("packer: flushing...")
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.currentBatch.Flush()
	p.currentBatch = newBatch(p.execute, p.debug)
	p.lastFlushTime = time.Now()
}

// LastFlushTime fn
func (p *Packer) LastFlushTime() time.Time {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.lastFlushTime
}
