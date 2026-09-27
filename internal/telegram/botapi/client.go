// Файл: internal/telegram/botapi/client.go
package botapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

var (
	ErrUnauthorized = errors.New("Bot API: токен не принят (401)")
	ErrConflict     = errors.New("Bot API: обновления уже получает кто-то другой (409)")
)

// Client — запросы к Bot API. Токен есть в адресе запроса: адрес не логируется и не попадает в ошибки.
type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string, hc *http.Client) *Client {
	return &Client{base: base, token: token, http: hc}
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var me User
	err := c.call(ctx, "getMe", url.Values{}, &me)
	return me, err
}

// GetUpdates — long polling: ждёт до timeoutSec секунд новые сообщения и правки.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	q := url.Values{}
	q.Set("offset", strconv.FormatInt(offset, 10))
	q.Set("timeout", strconv.Itoa(timeoutSec))
	q.Set("allowed_updates", `["message","edited_message"]`)
	ups := []Update{}
	err := c.call(ctx, "getUpdates", q, &ups)
	return ups, err
}

func (c *Client) call(ctx context.Context, method string, q url.Values, out any) error {
	endpoint := c.base + "/bot" + c.token + "/" + method + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("Bot API %s: запрос", method)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Bot API %s: %w", method, unwrapURL(err))
	}
	var env envelope
	decodeErr := json.NewDecoder(resp.Body).Decode(&env)
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("Bot API %s: закрыть ответ: %w", method, err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusConflict:
		return ErrConflict
	case decodeErr != nil:
		return fmt.Errorf("Bot API %s: ответ %d не разобран: %w", method, resp.StatusCode, decodeErr)
	case !env.OK:
		return fmt.Errorf("Bot API %s: %d %s", method, env.ErrorCode, env.Description)
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("Bot API %s: результат: %w", method, err)
	}
	return nil
}

// unwrapURL убирает из ошибки *url.Error адрес (в нём токен), оставляя причину.
func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
