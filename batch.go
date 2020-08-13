package packer

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"github.com/SevereCloud/vksdk/object"
)

type request struct {
	method  string
	params  api.Params
	handler func(api.Response, error)
}

type batch struct {
	requestID uint64
	callbacks map[string]request
	execute   func(code string) (packedExecuteResponse, error)
	debug     bool
}

func (b *batch) createRequestID() string {
	reqID := atomic.AddUint64(&b.requestID, 1)
	return "req" + strconv.FormatUint(reqID, 10)
}

func newBatch(exec func(code string) (packedExecuteResponse, error), debug bool) *batch {
	return &batch{
		callbacks: make(map[string]request),
		execute:   exec,
		debug:     debug,
	}
}

func (b *batch) appendRequest(req request) {
	requestID := b.createRequestID()
	b.callbacks[requestID] = req
}

func (b *batch) Flush() {
	if err := b.flush(); err != nil {
		for _, info := range b.callbacks {
			info.handler(api.Response{}, err)
		}
	}
}

func (b *batch) flush() error {
	packedResp, err := b.execute(b.code())
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for _, resp := range packedResp.Responses {
		info, ok := b.callbacks[resp.Key]
		if !ok {
			panic(fmt.Sprintf("packer: batch: handler for method %s not registered", info.method))
		}

		var err error
		if bytes.Equal(resp.Body, []byte("false")) {
			e := packedResp.Errors[failedRequestIndex]
			err = errors.New(executeErrorToMethodError(info, e))
			failedRequestIndex++
		}

		if b.debug {
			log.Printf("packer: batch: call handler %s (method %s): resp: %s, err: %s\n", resp.Key, info.method, resp.Body, err)
		}
		info.handler(api.Response{
			Response: resp.Body,
		}, err)
		delete(b.callbacks, resp.Key)
	}

	return nil
}

func (b *batch) code() string {
	var sb strings.Builder
	var responses []string
	for id, request := range b.callbacks {
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
		log.Printf("batch: code: \n%s\n", s)
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
