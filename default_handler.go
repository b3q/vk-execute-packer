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

const (
	// кол-во ретраев на запрос при TooMany
	tooManyMaxAttempts = 3
	// время хранения инфы о лимитах неиспользуемого токена
	tokenTTL = time.Minute * 1
	// периодичность проверки ttl токенов
	eraseCheckDuration = time.Minute * 5
)

type tokenInfo struct {
	lastUsed time.Time
	limiter  ratelimit.Limiter
}

type defaultHandler struct {
	rpsPerToken int
	rpscheck    map[string]tokenInfo
	mapLock     sync.Mutex
	client      *http.Client
	userAgent   string
	debug       bool
	stop        chan struct{}
}

func newDefaultHandler(rpsPerToken int) *defaultHandler {
	d := &defaultHandler{
		userAgent:   "vksdk",
		rpsPerToken: rpsPerToken,
		rpscheck:    make(map[string]tokenInfo),
		client: &http.Client{
			Timeout: time.Second * 10,
		},
	}

	go d.eraser()
	return d
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
		info, ok := def.rpscheck[tok]
		if !ok {
			info = tokenInfo{
				limiter: ratelimit.New(def.rpsPerToken),
			}
		}
		info.lastUsed = time.Now()
		limiter = info.limiter
		def.rpscheck[tok] = info
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

func (def *defaultHandler) eraser() {
	for {
		time.Sleep(eraseCheckDuration)
		select {
		case <-def.stop:
			return
		default:
			def.eraseUnusedTokens()
		}
	}
}

func (def *defaultHandler) eraseUnusedTokens() {
	def.mapLock.Lock()
	defer def.mapLock.Unlock()
	t := time.Now()
	for token, info := range def.rpscheck {
		if t.After(info.lastUsed.Add(tokenTTL)) {
			delete(def.rpscheck, token)
		}
	}
}

func (def *defaultHandler) Close() {
	def.stop <- struct{}{}
}
