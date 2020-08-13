package packer

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"github.com/SevereCloud/vksdk/object"
)

type taskState = uint64

const (
	filling taskState = iota
	working
)

type request struct {
	method  string
	params  api.Params
	handler func(api.Response, error)
}

type task struct {
	requestID uint64
	callbacks map[string]request
	mtx       sync.RWMutex
	execute   func(code string) (packedExecuteResponse, error)
	state     taskState
	debug     bool
}

func (task *task) createRequestID() string {
	reqID := atomic.AddUint64(&task.requestID, 1)
	return "req" + strconv.FormatUint(reqID, 10)
}

func newTask(exec func(code string) (packedExecuteResponse, error), debug bool) *task {
	return &task{
		callbacks: make(map[string]request),
		execute:   exec,
		state:     filling,
		debug:     debug,
	}
}

func (task *task) Request(method string, params api.Params, handler func(r api.Response, e error)) {
	if atomic.LoadUint64(&task.state) == working {
		panic("task: request called in working task")
	}

	task.mtx.Lock()
	defer task.mtx.Unlock()
	requestID := task.createRequestID()
	task.callbacks[requestID] = request{
		method:  method,
		params:  params,
		handler: handler,
	}
}

func (task *task) Flush() {
	if atomic.LoadUint64(&task.state) == working {
		return
	}
	atomic.StoreUint64(&task.state, working)
	task.mtx.Lock()
	defer task.mtx.Unlock()
	if err := task.flush(); err != nil {
		for _, info := range task.callbacks {
			info.handler(api.Response{}, err)
		}
	}
}

func (task *task) flush() error {
	packedResp, err := task.execute(task.code())
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for _, resp := range packedResp.Responses {
		info, ok := task.callbacks[resp.Key]
		if !ok {
			panic(fmt.Sprintf("packer: task: handler for method %s not registered", info.method))
		}

		var err error
		if bytes.Compare(resp.Body, []byte("false")) == 0 {
			e := packedResp.Errors[failedRequestIndex]
			err = errors.New(executeErrorToMethodError(info, e))
			failedRequestIndex++
		}

		if task.debug {
			log.Printf("packer: task: call handler %s (method %s): resp: %s, err: %s\n", resp.Key, info.method, resp.Body, err)
		}
		info.handler(api.Response{
			Response: resp.Body,
		}, err)
		delete(task.callbacks, resp.Key)
	}

	return nil
}

func (task *task) Len() int {
	task.mtx.RLock()
	defer task.mtx.RUnlock()
	return len(task.callbacks)
}

func (task *task) code() string {
	var sb strings.Builder
	var resps []string
	for id, request := range task.callbacks {
		sb.WriteString("var " + id + " = API." + request.method)
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

		s := "\"" + id + "\":" + id
		resps = append(resps, s)
	}

	sb.WriteString("return {")
	sb.WriteString(strings.Join(resps, ","))
	sb.WriteString("};")
	s := sb.String()

	if task.debug {
		log.Printf("task: code: \n%s\n", s)
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
