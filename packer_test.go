package packer

import (
	"testing"
	"time"

	"github.com/SevereCloud/vksdk/api"
	"github.com/stretchr/testify/assert"
)

func TestExecutionCode(t *testing.T) {
	p := NewPacker()
	go p.Handler("Account.getInfo", api.Params{
		"bar": 123,
	})
	time.Sleep(1 * time.Second)
	go p.Handler("Account.setInfo", api.Params{
		"bar": "abcdef",
	})
	time.Sleep(1 * time.Second)
	expected := "" +
		`var resp0 = API.Account.getInfo({"bar":123});` + "\n" +
		`var resp1 = API.Account.setInfo({"bar":"abcdef"});` + "\n" +
		`return {"resp0":resp0,"resp1":resp1};`

	assert.Equal(t, expected, p.requestsToCode())
}
