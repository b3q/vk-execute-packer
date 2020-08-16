package packer

import (
	"encoding/json"
	"log"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/object"
)

type packedExecuteResponse struct {
	Responses map[string]json.RawMessage
	Errors    []object.ExecuteError
}

type packedMethodResponse struct {
	Key  string
	Body []byte
}

func (p *Packer) execute(code string) (packedExecuteResponse, error) {
	apiResp, err := p.handler("execute", api.Params{
		"access_token": p.tokenPool.get(),
		"v":            api.Version,
		"code":         code,
	})
	if err != nil {
		return packedExecuteResponse{}, err
	}

	if p.debug {
		log.Printf("packer: execute: response: \n%s\n", apiResp.Response)
	}

	packedResp := packedExecuteResponse{
		Responses: make(map[string]json.RawMessage),
		Errors:    apiResp.ExecuteErrors,
	}

	if err := json.Unmarshal(apiResp.Response, &packedResp.Responses); err != nil {
		return packedResp, err
	}

	return packedResp, nil
}
