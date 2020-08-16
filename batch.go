package packer

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"github.com/SevereCloud/vksdk/object"
)

type request struct {
	method   string
	params   api.Params
	callback func(api.Response, error)
}

type batch struct {
	requestsNum int
	requests    map[string]request
	execute     func(code string) (packedExecuteResponse, error)
	debug       bool
}

func (b *batch) createRequestID() string {
	b.requestsNum++
	return "req" + strconv.Itoa(b.requestsNum)
}

func (b *batch) Count() int {
	return b.requestsNum
}

func newBatch(exec func(code string) (packedExecuteResponse, error), debug bool) *batch {
	return &batch{
		requests: make(map[string]request),
		execute:  exec,
		debug:    debug,
	}
}

func (b *batch) AppendRequest(req request) {
	requestID := b.createRequestID()
	b.requests[requestID] = req
}

func (b *batch) Send() {
	if err := b.send(); err != nil {
		for _, request := range b.requests {
			request.callback(api.Response{}, err)
		}
	}
}

func (b *batch) send() error {
	pack, err := b.execute(b.code())
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for name, body := range pack.Responses {
		request, ok := b.requests[name]
		if !ok {
			panic(fmt.Sprintf("packer: batch: handler %s (method %s) not registered", name, request.method))
		}

		var err error
		methodResponse := api.Response{
			Response: body,
		}
		if bytes.Equal(body, []byte("false")) {
			methodErr := executeErrorToMethodError(request, pack.Errors[failedRequestIndex])
			methodResponse.Error = methodErr
			err = errors.New(methodErr)
			failedRequestIndex++
		}

		if b.debug {
			log.Printf("packer: batch: call handler %s (method %s): resp: %s, err: %s\n", name, request.method, body, err)
		}

		request.callback(methodResponse, err)
		delete(b.requests, name)
	}

	if len(b.requests) > 0 {
		err := fmt.Errorf("packer: no response")
		for _, req := range b.requests {
			req.callback(api.Response{}, err)
		}
		b.requests = nil
	}

	return nil
}

func (b *batch) code() string {
	var sb strings.Builder
	var responses []string
	for id, request := range b.requests {
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
		responses = append(responses, s)
	}

	sb.WriteString("return {" + strings.Join(responses, ",") + "};")
	s := sb.String()
	if b.debug {
		log.Printf("packer: batch: code: \n%s\n", s)
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
