package packer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/SevereCloud/vksdk/api"
	"github.com/SevereCloud/vksdk/api/errors"
	"go.uber.org/ratelimit"
)

// кол-во ретраев на запрос при TooMany
const tooManyMaxAttempts = 3

type baseHandler struct {
	rpsPerToken int
	rpscheck    map[string]ratelimit.Limiter
	mapLock     sync.Mutex
	client      *http.Client
	debug       bool
}

func newBaseHandler(rpsPerToken int) *baseHandler {
	return &baseHandler{
		rpsPerToken: rpsPerToken,
		rpscheck:    make(map[string]ratelimit.Limiter),
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

func (base *baseHandler) Handle(method string, params api.Params) (api.Response, error) {
	var (
		u        = api.APIMethodURL + method
		query    = &url.Values{}
		attempts = 0
		apiResp  api.Response
		limiter  ratelimit.Limiter
	)
	for key, value := range params {
		query.Set(key, api.FmtValue(value, 0))
	}

	if tokIface, ok := params["access_token"]; !ok {
		limiter = ratelimit.NewUnlimited()
	} else {
		tok, ok := tokIface.(string)
		if !ok {
			panic("packer: baseHandler: bad access_token type")
		}

		base.mapLock.Lock()
		limiter, ok = base.rpscheck[tok]
		if !ok {
			limiter = ratelimit.New(base.rpsPerToken)
			base.rpscheck[tok] = limiter
		}
		base.mapLock.Unlock()
	}

	for {
		limiter.Take()
		attempts++

		req, err := http.NewRequest(http.MethodPost, u, bytes.NewBufferString(query.Encode()))
		if err != nil {
			return apiResp, err
		}
		req.Header.Set("User-Agent", "boo")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := base.client.Do(req)
		if err != nil {
			return apiResp, err
		}
		defer resp.Body.Close()

		if base.debug {
			bodyBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return apiResp, err
			}

			log.Printf("packer: %s response:\n%s\n", method, bodyBytes)
			if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
				return apiResp, err
			}
		} else {
			if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
				return apiResp, err
			}
		}

		mediatype, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if mediatype != "application/json" {
			return apiResp, fmt.Errorf("invalid content-type")
		}

		if err := errors.New(apiResp.Error); err != nil {
			if errors.GetType(err) == errors.TooMany && attempts < tooManyMaxAttempts {
				continue
			}
			return apiResp, err
		}

		return apiResp, nil
	}
}
