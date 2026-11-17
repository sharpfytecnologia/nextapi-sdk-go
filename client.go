// Package nextapi é o SDK oficial da NextAPI (CNPJ, CEP, sócios, mercado). Só usa a biblioteca padrão.
//
//	client, err := nextapi.NewClient() // lê NEXTAPI_KEY
//	empresa, err := client.Cnpj.Consultar(ctx, "11.222.333/0001-81", nil)
//	if errors.Is(err, nextapi.ErrNotFound) { ... }
package nextapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version do SDK.
const Version = "0.2.0"

// DefaultBaseURL é o endereço da API, o mesmo para produção e sandbox (a API decide pela chave).
const DefaultBaseURL = "https://api.nextapi.com.br"

// Client da NextAPI. Seguro para uso concorrente: crie um e reutilize.
type Client struct {
	apiKey     string
	baseURL    string
	timeout    time.Duration
	maxRetries int
	userAgent  string
	appInfo    string
	clientInfo string
	http       *http.Client

	// Environment é "sandbox" para chaves nxa_test_ e "production" para as demais.
	Environment string

	Cnpj       *CnpjService
	Socios     *SociosService
	Ceps       *CepsService
	Empresas   *EmpresasService
	Cnaes      *CnaesService
	Estados    *EstadosService
	Municipios *MunicipiosService
	Mercado    *MercadoService
	Creditos   *CreditosService
}

// Option configura o cliente.
type Option func(*Client)

// WithAPIKey define a chave (nxa_live_… ou nxa_test_…). Padrão: NEXTAPI_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL troca o endereço da API; só https (http apenas para localhost).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithTimeout define o tempo máximo de cada tentativa (padrão 30 s).
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithMaxRetries define as novas tentativas em falha transitória (429, 502, 503, 504, rede), só em GET. Padrão 2.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = max(0, n) } }

// WithHTTPClient usa um http.Client próprio (proxy, TLS). O SDK ainda recusa redirecionamentos.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithAppInfo acrescenta a sua aplicação ao User-Agent (ex.: "minha-app/1.2").
func WithAppInfo(info string) Option {
	return func(c *Client) { c.appInfo = info }
}

// NewClient cria o cliente.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL:    DefaultBaseURL,
		timeout:    30 * time.Second,
		maxRetries: 2,
	}
	for _, o := range opts {
		o(c)
	}
	app := sanitizeAppInfo(c.appInfo)
	rt := "go/" + strings.TrimPrefix(runtime.Version(), "go")
	c.userAgent = fmt.Sprintf("nextapi-go/%s %s", Version, rt)
	c.clientInfo = rt + "; " + osName(runtime.GOOS)
	if app != "" {
		c.userAgent += " " + app
		c.clientInfo += "; " + app
	}
	if c.apiKey == "" {
		c.apiKey = os.Getenv("NEXTAPI_KEY")
	}
	c.apiKey = strings.TrimSpace(c.apiKey)
	if c.apiKey == "" {
		return nil, &Error{Kind: KindAuthentication, Message: "chave de API não informada: use WithAPIKey ou defina NEXTAPI_KEY"}
	}
	base, err := checkBaseURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	c.baseURL = base
	c.Environment = "production"
	if strings.HasPrefix(c.apiKey, "nxa_test_") {
		c.Environment = "sandbox"
	}

	// Cópia do http.Client para não mexer no do chamador; nunca segue redirecionamento (a chave iria junto).
	hc := http.Client{}
	if c.http != nil {
		hc = *c.http
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.http = &hc

	c.Cnpj = &CnpjService{c}
	c.Socios = &SociosService{c}
	c.Ceps = &CepsService{c}
	c.Empresas = &EmpresasService{c}
	c.Cnaes = &CnaesService{c}
	c.Estados = &EstadosService{c}
	c.Municipios = &MunicipiosService{c}
	c.Mercado = &MercadoService{c}
	c.Creditos = &CreditosService{c}
	return c, nil
}

// String nunca mostra a chave.
func (c *Client) String() string {
	return fmt.Sprintf("nextapi.Client{BaseURL: %s, Environment: %s}", c.baseURL, c.Environment)
}

// GoString nunca mostra a chave (fmt %#v).
func (c *Client) GoString() string { return c.String() }

func checkBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("nextapi: base URL inválida: %s", raw)
	}
	h := u.Hostname()
	local := h == "localhost" || h == "127.0.0.1" || h == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("nextapi: a base URL tem de usar https (http só para localhost): a chave vai no cabeçalho")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/"), nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	// POST de lote consome créditos: nunca repete sozinho.
	retries := 0
	if method == http.MethodGet {
		retries = c.maxRetries
	}

	for attempt := 0; ; attempt++ {
		status, header, data, err := c.once(ctx, method, path, payload)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < retries {
				if werr := sleep(ctx, backoff(attempt+1)); werr != nil {
					return werr
				}
				continue
			}
			return err
		}
		if status >= 200 && status < 300 {
			if out == nil || len(data) == 0 || status == http.StatusNoContent {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return &Error{Kind: KindServer, Status: status, Message: "resposta inválida da NextAPI: " + err.Error(), Body: data, Err: err}
			}
			return nil
		}
		if status >= 300 && status < 400 {
			return &Error{Kind: KindConnection, Status: status,
				Message: fmt.Sprintf("a NextAPI respondeu com redirecionamento (%d); o SDK não o segue para não reenviar a chave", status)}
		}
		wait, hasWait := retryAfter(header.Get("Retry-After"))
		if (status == 429 || status == 502 || status == 503 || status == 504) && attempt < retries {
			if !hasWait {
				wait = backoff(attempt + 1)
			}
			if werr := sleep(ctx, wait); werr != nil {
				return werr
			}
			continue
		}
		return toError(status, header, data, wait)
	}
}

func (c *Client) once(ctx context.Context, method, path string, payload []byte) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, nil, &Error{Kind: KindConnection, Message: err.Error(), Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-NextApi-Client", "go/"+Version)
	req.Header.Set("X-NextApi-Client-Info", c.clientInfo)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		msg := "falha de conexão com a NextAPI"
		if errors.Is(err, context.DeadlineExceeded) {
			msg = fmt.Sprintf("sem resposta da NextAPI em %d ms", c.timeout.Milliseconds())
		}
		// A mensagem de *url.Error tem só método e URL, sem cabeçalhos: a chave não vaza.
		return 0, nil, nil, &Error{Kind: KindConnection, Message: msg, Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return 0, nil, nil, &Error{Kind: KindConnection, Message: "resposta interrompida", Err: err}
	}
	return resp.StatusCode, resp.Header, data, nil
}

func toError(status int, header http.Header, data []byte, wait time.Duration) error {
	var fields struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &fields)
	msg := fields.Message
	if msg == "" {
		msg = "HTTP " + strconv.Itoa(status)
	}
	return &Error{Kind: kindOf(status, fields.Code), Status: status, Code: fields.Code, Message: msg, Body: data,
		RequestID: header.Get("X-Request-Id"), RetryAfter: wait}
}

func retryAfter(h string) (time.Duration, bool) {
	if h == "" {
		return 0, false
	}
	var d time.Duration
	if s, err := strconv.ParseFloat(h, 64); err == nil {
		d = time.Duration(s * float64(time.Second))
	} else if t, err := http.ParseTime(h); err == nil {
		d = time.Until(t)
	} else {
		return 0, false
	}
	return min(max(d, 0), 60*time.Second), true
}

func backoff(attempt int) time.Duration {
	base := min(8*time.Second, 500*time.Millisecond<<(attempt-1))
	return base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// sanitizeAppInfo deixa só caracteres seguros num cabeçalho: CR/LF ou espaço no appInfo quebrariam a requisição.
func sanitizeAppInfo(info string) string {
	var b strings.Builder
	for _, r := range info {
		if b.Len() >= 64 {
			break
		}
		if r < 128 && (r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("._/@+-", r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func osName(goos string) string {
	switch goos {
	case "linux", "darwin", "windows":
		return goos
	}
	return "other"
}
