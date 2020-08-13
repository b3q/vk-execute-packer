package packer

import (
	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"github.com/SevereCloud/vksdk/object"
	"github.com/tidwall/gjson"
)

type packedExecuteResponse struct {
	Responses []packedMethodResponse
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

	if err := errors.New(apiResp.Error); err != nil {
		return packedExecuteResponse{}, err
	}

	packedResp := packedExecuteResponse{
		Errors: apiResp.ExecuteErrors,
	}

	gjson.ParseBytes(apiResp.Response).ForEach(func(key, value gjson.Result) bool {
		packedResp.Responses = append(packedResp.Responses, packedMethodResponse{
			Key:  key.String(),
			Body: []byte(value.Raw),
		})
		return true
	})

	return packedResp, nil
}
