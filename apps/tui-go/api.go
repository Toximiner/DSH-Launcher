package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

type dshAPI struct {
	origin string
	client *http.Client
}

func openAPI(ctx context.Context, login string) (*dshAPI, error) {
	u, err := settingsURL(login)
	if err != nil {
		return nil, err
	}
	jar, _ := cookiejar.New(nil)
	api := &dshAPI{origin: u.Scheme + "://" + u.Host, client: &http.Client{Jar: jar, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	q := u.Query()
	q.Set("interactions", "answer")
	u.Path = "/connect"
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	response, err := api.client.Do(req)
	if err != nil {
		api.close()
		return nil, errors.New("не удалось подключиться к dsh")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound || len(jar.Cookies(u)) == 0 {
		api.close()
		return nil, fmt.Errorf("вход в dsh не выполнен (HTTP %d); получите свежий URL входа", response.StatusCode)
	}
	return api, nil
}
func (api *dshAPI) close() { api.client.CloseIdleConnections() }

func (api *dshAPI) rpc(ctx context.Context, method string, args map[string]any, target any) error {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return errors.New("не удалось создать идентификатор запроса")
	}
	rpcID := hex.EncodeToString(id)
	body, err := json.Marshal(map[string]any{"type": "client-request", "rpcId": rpcID, "method": method, "payload": map[string]any{"args": args}})
	if err != nil {
		return errors.New("не удалось подготовить запрос dsh")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, api.origin+"/api/"+method, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", api.origin)
	response, err := api.client.Do(req)
	if err != nil {
		return errors.New("запрос dsh не выполнен; проверьте подключение")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("dsh: %s недоступен (HTTP %d)", method, response.StatusCode)
	}
	const limit = 8 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit {
		return errors.New("ответ dsh слишком велик или не прочитан")
	}
	var envelope struct {
		Type   string `json:"type"`
		ID     string `json:"rpcId"`
		Result struct {
			OK    *bool           `json:"ok"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Type != "server-response" || envelope.ID != rpcID || envelope.Result.OK == nil {
		return errors.New("неподдерживаемый ответ API dsh")
	}
	if !*envelope.Result.OK {
		return fmt.Errorf("dsh отказал в операции %s", method)
	}
	if target != nil {
		if len(envelope.Result.Value) == 0 || json.Unmarshal(envelope.Result.Value, target) != nil {
			return errors.New("неподдерживаемые данные API dsh")
		}
	}
	return nil
}
