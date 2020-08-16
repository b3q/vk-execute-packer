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

type batch map[string]request

func (b batch) appendRequest(req request) {
	b["req"+strconv.Itoa(len(b))] = req
}

func (b batch) code() string {
	var sb strings.Builder
	var responses []string
	for id, request := range b {
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
	return sb.String()
}

func (p *Packer) sendBatch(bat batch) {
	if err := p.trySendBatch(bat); err != nil {
		for _, request := range bat {
			request.callback(api.Response{}, err)
		}
	}
}

func (p *Packer) trySendBatch(bat batch) error {
	pack, err := p.execute(bat.code())
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for name, body := range pack.Responses {
		request, ok := bat[name]
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

		if p.debug {
			log.Printf("packer: batch: call handler %s (method %s): resp: %s, err: %s\n", name, request.method, body, err)
		}

		request.callback(methodResponse, err)
		delete(bat, name)
	}

	if len(bat) > 0 {
		err := fmt.Errorf("packer: no response")
		for _, req := range bat {
			req.callback(api.Response{}, err)
		}
		bat = nil
	}

	return nil
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
