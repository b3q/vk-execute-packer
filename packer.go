package packer

import (
	"log"
	"sync"
	"sync/atomic"
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
	lastFlushTimeUnix int64
	maxPackedRequests int
	tokenPool         *tokenPool
	tokenLazyLoading  bool
	filterMode        FilterMode
	filterMethods     map[string]struct{}
	debug             bool
	handler           func(string, api.Params) (api.Response, error)
	requests          chan request
	forceFlush        chan struct{}
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
		lastFlushTimeUnix: time.Now().Unix(),
		maxPackedRequests: 25,
		filterMode:        Ignore,
		filterMethods:     make(map[string]struct{}),
		handler:           vk.Handler,
		requests:          make(chan request, 10),
		forceFlush:        make(chan struct{}),
	}
	vk.Handler = p.Handler

	for _, opt := range opts {
		opt(p)
	}

	go p.worker()
	return p
}

// Default func
func Default(vk *api.VK, opts ...Option) {
	p := New(vk, opts...)
	go TimeoutTrigger(time.Second*2, p)
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
	p.requests <- request{method, params, handler}
	wg.Wait()
	return resp, err
}

func (p *Packer) worker() {
	batch := newBatch(p.execute, p.debug)
	requestsCount := 0
	for {
		select {
		case req := <-p.requests:
			batch.appendRequest(req)
			requestsCount++
			if requestsCount == p.maxPackedRequests {
				if p.debug {
					log.Println("packer: sending batch...")
				}
				go batch.Flush()
				batch = newBatch(p.execute, p.debug)
				requestsCount = 0
			}
		case <-p.forceFlush:
			if requestsCount == 0 {
				continue
			}
			if p.debug {
				log.Println("packer: forced sending batch...")
			}
			go batch.Flush()
			batch = newBatch(p.execute, p.debug)
			requestsCount = 0
		}
	}
}

// Flush func
func (p *Packer) Flush() {
	p.forceFlush <- struct{}{}
	atomic.StoreInt64(&p.lastFlushTimeUnix, time.Now().Unix())
}

// LastFlushTime fn
func (p *Packer) LastFlushTime() time.Time {
	return time.Unix(atomic.LoadInt64(&p.lastFlushTimeUnix), 0)
}
