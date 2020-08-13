package packer

import (
	"testing"
	"time"

	"github.com/SevereCloud/vksdk/api"
	"github.com/stretchr/testify/assert"
)

func TestExecutionCode(t *testing.T) {
	b := newBatch(func(code string) (packedExecuteResponse, error) {
		return packedExecuteResponse{}, nil
	}, true)
	go b.appendRequest(request{
		method: "Account.getInfo",
		params: api.Params{
			"bar": 123,
		},
	})
	time.Sleep(1 * time.Second)
	go b.appendRequest(request{
		method: "Account.setInfo",
		params: api.Params{
			"bar": "abcdef",
		},
	})
	time.Sleep(1 * time.Second)
	expected := "" +
		`var resp0 = API.Account.getInfo({"bar":123});` + "\n" +
		`var resp1 = API.Account.setInfo({"bar":"abcdef"});` + "\n" +
		`return {"resp0":resp0,"resp1":resp1};`

	assert.Equal(t, expected, b.code())
}
