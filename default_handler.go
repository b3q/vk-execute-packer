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

type defaultHandler struct {
	rpsPerToken int
	rpscheck    map[string]ratelimit.Limiter
	mapLock     sync.Mutex
	client      *http.Client
	userAgent   string
	debug       bool
}

func newDefaultHandler(rpsPerToken int) *defaultHandler {
	return &defaultHandler{
		userAgent:   "vksdk",
		rpsPerToken: rpsPerToken,
		rpscheck:    make(map[string]ratelimit.Limiter),
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}
}

func (def *defaultHandler) Handle(method string, params api.Params) (api.Response, error) {
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
			panic("packer: defaultHandler: bad access_token type")
		}

		def.mapLock.Lock()
		limiter, ok = def.rpscheck[tok]
		if !ok {
			limiter = ratelimit.New(def.rpsPerToken)
			def.rpscheck[tok] = limiter
		}
		def.mapLock.Unlock()
	}

retry:
	limiter.Take()
	attempts++

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewBufferString(query.Encode()))
	if err != nil {
		return apiResp, err
	}
	req.Header.Set("User-Agent", def.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := def.client.Do(req)
	if err != nil {
		return apiResp, err
	}
	defer resp.Body.Close()

	if def.debug {
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
			goto retry
		}
		return apiResp, err
	}

	return apiResp, nil

}
