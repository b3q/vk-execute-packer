package packer

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"github.com/SevereCloud/vksdk/object"
)

type request struct {
	method  string
	params  api.Params
	handler func(api.Response, error)
}

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
	requestID         int
	handlers          map[string]request
	filterMode        FilterMode
	filterMethods     map[string]struct{}
	mtx               sync.RWMutex
	debug             bool
	handler           func(string, api.Params) (api.Response, error)
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

// NewPacker ...
func NewPacker(vk *api.VK, flusher Flusher, opts ...Option) *Packer {
	p := &Packer{
		tokenLazyLoading:  true,
		tokenPool:         newTokenPool(),
		lastFlushTime:     time.Now(),
		maxPackedRequests: 25,
		handlers:          make(map[string]request),
		filterMode:        Ignore,
		filterMethods: map[string]struct{}{
			"execute": {},
		},
		handler: vk.Handler,
	}

	for _, opt := range opts {
		opt(p)
	}

	go flusher(p)

	return p
}

// Default func
func Default(vk *api.VK, opts ...Option) {
	p := NewPacker(
		vk,
		TimeoutBasedFlusher(time.Second*1),
		opts...,
	)
	vk.Handler = p.Handler
}

// Handler func
func (p *Packer) Handler(method string, params api.Params) (api.Response, error) {
	if p.debug {
		log.Printf("packer: Handler call (%s)\n", method)
	}

	{
		if method == "execute" {
			return p.handler(method, params)
		}
		_, found := p.filterMethods[method]
		if (p.filterMode == Allow && !found) ||
			(p.filterMode == Ignore && found) {
			return p.handler(method, params)
		}
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

	p.mtx.Lock()
	requestID := p.requestID
	p.requestID++

	handler := func(r api.Response, e error) {
		resp = r
		err = e
		wg.Done()
	}

	p.handlers["resp"+strconv.Itoa(requestID)] = request{
		method:  method,
		params:  params,
		handler: handler,
	}

	needFlush := len(p.handlers) == p.maxPackedRequests
	p.mtx.Unlock()

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
	defer func() {
		p.handlers = make(map[string]request)
		p.requestID = 0
		p.lastFlushTime = time.Now()
		p.mtx.Unlock()
	}()

	if err := p.flush(); err != nil {
		for _, info := range p.handlers {
			info.handler(api.Response{}, err)
		}
	}
}

func (p *Packer) flush() error {
	packedResp, err := p.execute(p.tokenPool.get(), p.requestsToCode())
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for _, resp := range packedResp.Responses {
		info, ok := p.handlers[resp.Key]
		if !ok {
			panic(fmt.Sprintf("packer: handler for method %s not registered", info.method))
		}

		var err error
		if bytes.Compare(resp.Body, []byte("false")) == 0 {
			e := packedResp.Errors[failedRequestIndex]
			err = errors.New(executeErrorToMethodError(info, e))
			failedRequestIndex++
		}

		if p.debug {
			log.Printf("packer: call handler: %s, (resp: %s, err: %s)\n", info.method, resp.Body, err)
		}
		info.handler(api.Response{
			Response: resp.Body,
		}, err)
		delete(p.handlers, resp.Key)
	}

	return nil
}

// LastFlushTime fn
func (p *Packer) LastFlushTime() time.Time {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.lastFlushTime
}

func (p *Packer) requestsToCode() string {
	var sb strings.Builder
	requestIndex := 0
	for _, request := range p.handlers {
		sb.WriteString("var resp" + strconv.Itoa(requestIndex) + " = API." + request.method)
		sb.WriteString("({")
		var params []string
		for name, value := range request.params {
			var fmted string
			if s, ok := value.(string); ok {
				fmted = `"` + s + `"`
			} else {
				fmted = api.FmtValue(value, 0)
			}

			s := "\"" + name + "\":" + fmted
			params = append(params, s)
		}
		sb.WriteString(strings.Join(params, ","))
		sb.WriteString("});\n")
		requestIndex++
	}

	sb.WriteString("return {")
	var resps []string
	for i := 0; i < requestIndex; i++ {
		s := "\"resp" + strconv.Itoa(i) + "\":" + "resp" + strconv.Itoa(i)
		resps = append(resps, s)
	}

	sb.WriteString(strings.Join(resps, ","))
	sb.WriteString("};")
	s := sb.String()

	if p.debug {
		log.Printf("packer: code:\n%s\n", s)
	}
	return s
}

func executeErrorToMethodError(req request, err object.ExecuteError) object.Error {
	params := make([]object.BaseRequestParam, len(req.params))
	for key, value := range req.params {
		params = append(params, object.BaseRequestParam{
			Key:   key,
			Value: api.FmtValue(value, 0),
		})
	}

	return object.Error{
		Message:       err.ErrorMsg,
		Code:          err.ErrorCode,
		RequestParams: params,
	}
}
