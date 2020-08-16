package packer

import (
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SevereCloud/vksdk/api"
)

// FilterMode - batch filter mode
type FilterMode bool

const (
	// Allow mode
	Allow FilterMode = true
	// Ignore mode
	Ignore FilterMode = false
)

// Packer struct
type Packer struct {
	lastSendTimeUnix  int64
	maxPackedRequests int
	tokenPool         *tokenPool
	tokenLazyLoading  bool
	filterMode        FilterMode
	filterMethods     map[string]struct{}
	debug             bool
	handler           func(string, api.Params) (api.Response, error)
	batch             batch
	mtx               sync.Mutex
}

// Option - Packer option
type Option func(*Packer)

// MaxPackedRequests sets the maximum API calls inside one batch.
func MaxPackedRequests(max int) Option {
	if max < 1 || max > 25 {
		max = 25
	}
	return func(p *Packer) {
		p.maxPackedRequests = max
	}
}

// Rules sets the batching rules (ignore some methods or allow it).
func Rules(mode FilterMode, methods ...string) Option {
	return func(p *Packer) {
		for _, m := range methods {
			p.filterMode = mode
			p.filterMethods[m] = struct{}{}
		}
	}
}

// Debug enables printing debug info into stdout.
func Debug() Option {
	return func(p *Packer) {
		p.debug = true
	}
}

// Tokens provides tokens which will be used for sending batch requests.
// If tokens are not provided, packer will use tokens from incoming requests.
func Tokens(tokens ...string) Option {
	return func(p *Packer) {
		p.tokenLazyLoading = false
		p.tokenPool = newTokenPool(tokens...)
	}
}

// New creates a new Packer.
// Also automatically wraps vk.Handler with their own (for batching requests).
//
// NOTE: this method will not create any trigger for sending batches
// which means that the batch will be sent only when the number of requests in it
// equals to 'maxPackedRequests' (default 25, can be overwritten with MaxPackedRequests() option).
// You will need to use TimeoutTrigger or create your custom trigger
// which calls packer.Send() method to solve this.
func New(vk *api.VK, opts ...Option) *Packer {
	p := &Packer{
		tokenLazyLoading:  true,
		tokenPool:         newTokenPool(),
		lastSendTimeUnix:  time.Now().Unix(),
		maxPackedRequests: 25,
		filterMode:        Ignore,
		filterMethods:     make(map[string]struct{}),
		handler:           vk.Handler,
		batch:             make(batch),
	}
	vk.Handler = p.Handler
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Default creates new Packer, wraps vk.Handler and creates
// timeout-based trigger for sending batches every 2 seconds.
func Default(vk *api.VK, opts ...Option) {
	p := New(vk, opts...)
	go TimeoutTrigger(time.Second*2, p)
}

// Handler implements vk.Handler function, which proceeds requests to VK API.
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

	p.mtx.Lock()
	p.batch["req"+strconv.Itoa(len(p.batch))] = request{method, params, handler}
	if len(p.batch) == p.maxPackedRequests {
		go p.sendBatch(p.batch)
		p.batch = make(batch)
	}
	p.mtx.Unlock()

	wg.Wait()
	return resp, err
}

// Send triggers to send current batch.
func (p *Packer) Send() {
	p.mtx.Lock()
	if len(p.batch) > 0 {
		go p.sendBatch(p.batch)
		p.batch = make(batch)
	}
	p.mtx.Unlock()
}

// LastSendTime returns time of last sent batch.
func (p *Packer) LastSendTime() time.Time {
	return time.Unix(atomic.LoadInt64(&p.lastSendTimeUnix), 0)
}
