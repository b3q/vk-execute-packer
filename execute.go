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

func (p *Packer) execute(code string) (packedExecuteResponse, error) {
	resp, err := p.vkHandler("execute", api.Params{
		"access_token": p.tokenPool.Get(),
		"v":            api.Version,
		"code":         code,
	})
	if err != nil {
		return packedExecuteResponse{}, err
	}

	if p.debug {
		log.Printf("packer: execute: response: \n%s\n", resp.Response)
	}

	packed := packedExecuteResponse{
		Responses: make(map[string]json.RawMessage),
		Errors:    resp.ExecuteErrors,
	}

	if err := json.Unmarshal(resp.Response, &packed.Responses); err != nil {
		return packed, err
	}

	return packed, nil
}
