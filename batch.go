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
	b["r"+strconv.Itoa(len(b))] = req
}

func (b batch) code() string {
	var sb strings.Builder
	sb.WriteString("return {")
	for id, request := range b {
		sb.WriteString(`"` + id + `":API.` + request.method + "({")
		for name, value := range request.params {
			if name == "access_token" {
				continue
			}
			valueString := ""
			if s, ok := value.(string); ok {
				valueString = `"` + s + `"`
			} else {
				valueString = api.FmtValue(value, 0)
			}
			sb.WriteString(`"` + name + `":` + valueString + ",")
		}
		sb.WriteString("}),")
	}
	sb.WriteString("};")
	return sb.String()
}

func (b batch) forEach(iter func(request)) {
	keys := make([]string, 0, len(b))
	for key := range b {
		keys = append(keys, key)
	}

	for _, key := range keys {
		req := b[key]
		iter(req)
	}
}

func (b batch) split(num int) []batch {
	requestsPerBatch := len(b) / num
	batches := make([]batch, len(b)/requestsPerBatch)

	index := 0
	count := 0
	b.forEach(func(r request) {
		tmp := batches[index]
		if tmp == nil {
			tmp = make(batch)
		}

		tmp.appendRequest(r)
		count++
		if count == requestsPerBatch {
			index++
			count = 0
		}
	})

	return batches
}

func (p *Packer) sendBatch(bat batch) {
	err := p.trySendBatch(bat)
	if err.Error() == "response size is too big" {
		for _, splitBatch := range bat.split(2) {
			go p.sendBatch(splitBatch)
		}
		return
	}

	if err != nil {
		for _, request := range bat {
			request.callback(api.Response{}, err)
		}
	}
}

func (p *Packer) trySendBatch(bat batch) error {
	code := bat.code()
	if p.debug {
		log.Printf("packer: batch: code: \n%s\n", code)
	}

	pack, err := p.execute(code)
	if err != nil {
		return err
	}

	failedRequestIndex := 0
	for name, body := range pack.Responses {
		request, ok := bat[name]
		if !ok {
			if p.debug {
				log.Printf("packer: batch: handler %s not registered\n", name)
			}
			continue
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
