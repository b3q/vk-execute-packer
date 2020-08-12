package packer

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

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

func (p *Packer) execute(token, code string) (packedExecuteResponse, error) {
	u := api.APIMethodURL + "execute"
	query := &url.Values{}
	query.Set("access_token", token)
	query.Set("v", api.Version)
	query.Set("code", code)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBufferString(query.Encode()))
	if err != nil {
		return packedExecuteResponse{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return packedExecuteResponse{}, err
	}
	defer resp.Body.Close()

	var apiResp api.Response
	if p.debug {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return packedExecuteResponse{}, err
		}

		log.Printf("packer: code: \n%s\n", code)
		log.Printf("packer: response: \n%s\n", bodyBytes)
		if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
			return packedExecuteResponse{}, err
		}
	} else {
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return packedExecuteResponse{}, err
		}
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
