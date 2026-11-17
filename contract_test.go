package nextapi

// Contrato compartilhado pelos seis SDKs (sdks/contract/cases.json).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const testKey = "nxa_test_0123456789abcdef"

// contractPath acha o contrato em sdks/contract (monorepo) ou em contract/ dentro do SDK (espelho público).
func contractPath(name string) string {
	if _, err := os.Stat("contract/" + name); err == nil {
		return "contract/" + name
	}
	return "../contract/" + name
}

type contractFile struct {
	Cases []struct {
		ID       string            `json:"id"`
		Call     string            `json:"call"`
		Args     []json.RawMessage `json:"args"`
		Method   string            `json:"method"`
		Path     string            `json:"path"`
		Query    map[string]string `json:"query"`
		Body     json.RawMessage   `json:"body"`
		Response json.RawMessage   `json:"response"`
	} `json:"cases"`
	Errors []struct {
		Status    int             `json:"status"`
		Body      json.RawMessage `json:"body"`
		Error     string          `json:"error"`
		Retryable bool            `json:"retryable"`
	} `json:"errors"`
	Invalid []struct {
		Call   string            `json:"call"`
		Args   []json.RawMessage `json:"args"`
		Reason string            `json:"reason"`
	} `json:"invalid"`
	Valid []struct {
		Call string            `json:"call"`
		Args []json.RawMessage `json:"args"`
		Path string            `json:"path"`
	} `json:"valid"`
	Headers struct {
		Client  string `json:"client"`
		Info    string `json:"info"`
		AppInfo struct {
			Input     string `json:"input"`
			Sanitized string `json:"sanitized"`
		} `json:"appInfo"`
	} `json:"headers"`
}

func loadContract(t *testing.T) contractFile {
	data, err := os.ReadFile(contractPath("cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c contractFile
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

type seen struct {
	req  *http.Request
	body string
}

type fakeRT struct {
	seen    []seen
	respond func(n int) (*http.Response, error)
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	var b string
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		b = string(data)
	}
	f.seen = append(f.seen, seen{r, b})
	return f.respond(len(f.seen))
}

func jsonResp(status int, body string, headers map[string]string) *http.Response {
	h := http.Header{"Content-Type": {"application/json"}}
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func newTestClient(t *testing.T, rt http.RoundTripper, opts ...Option) *Client {
	t.Helper()
	c, err := NewClient(append([]Option{WithAPIKey(testKey), WithHTTPClient(&http.Client{Transport: rt}), WithMaxRetries(2)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type obj map[string]any

func (o obj) s(k string) string {
	if v, ok := o[k]; ok {
		return fmt.Sprint(v)
	}
	return ""
}
func (o obj) i(k string) int {
	if v, ok := o[k].(float64); ok {
		return int(v)
	}
	return 0
}
func (o obj) f(k string) float64 { v, _ := o[k].(float64); return v }
func (o obj) ip(k string) *int {
	if _, ok := o[k]; !ok {
		return nil
	}
	return Int(o.i(k))
}
func (o obj) bp(k string) *bool {
	if v, ok := o[k].(bool); ok {
		return &v
	}
	return nil
}

// invoke traduz a chamada canônica (camelCase, argumentos em JSON) para a API Go.
func invoke(c *Client, call string, raw []json.RawMessage) error {
	ctx := context.Background()
	var str string
	var list []string
	var num int
	o := obj{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw[0], &str)
		_ = json.Unmarshal(raw[0], &list)
		_ = json.Unmarshal(raw[0], &num)
		_ = json.Unmarshal(raw[len(raw)-1], &o)
	}
	pag := &PaginaOptions{Page: o.i("page"), PageSize: o.i("pageSize"), IncluirInativas: o.bp("incluirInativas")}
	lista := &ListaOptions{Page: o.i("page"), PageSize: o.i("pageSize")}
	geo := &GeoOptions{Page: o.i("page"), PageSize: o.i("pageSize")}
	var err error
	switch call {
	case "cnpj.consultar":
		_, err = c.Cnpj.Consultar(ctx, str, &ConsultaOptions{ToleranceDays: o.ip("toleranceDays")})
	case "cnpj.consultarAoVivo":
		_, err = c.Cnpj.ConsultarAoVivo(ctx, str)
	case "cnpj.obterSimples":
		_, err = c.Cnpj.ObterSimples(ctx, str, &ConsultaOptions{ToleranceDays: o.ip("toleranceDays")})
	case "cnpj.obterInscricoesEstaduais":
		_, err = c.Cnpj.ObterInscricoesEstaduais(ctx, str)
	case "cnpj.obterConformidade":
		_, err = c.Cnpj.ObterConformidade(ctx, str)
	case "cnpj.obterRelacionamentos":
		_, err = c.Cnpj.ObterRelacionamentos(ctx, str, pag)
	case "cnpj.consultarLote":
		_, err = c.Cnpj.ConsultarLote(ctx, list)
	case "cnpj.obterSimplesLote":
		_, err = c.Cnpj.ObterSimplesLote(ctx, list)
	case "socios.listarEmpresas":
		_, err = c.Socios.ListarEmpresas(ctx, str, pag)
	case "ceps.consultar":
		_, err = c.Ceps.Consultar(ctx, str)
	case "ceps.buscar":
		_, err = c.Ceps.Buscar(ctx, CepFiltro{Uf: o.s("uf"), IdMunicipio: o.i("idMunicipio"), NomeMunicipio: o.s("nomeMunicipio"),
			Logradouro: o.s("logradouro"), Q: o.s("q"), Page: o.i("page"), PageSize: o.i("pageSize")})
	case "ceps.buscarProximos":
		_, err = c.Ceps.BuscarProximos(ctx, o.f("latitude"), o.f("longitude"), o.f("raioMetro"), geo)
	case "ceps.buscarProximosDoCep":
		_, err = c.Ceps.BuscarProximosDoCep(ctx, str, o.f("raioMetro"), geo)
	case "empresas.pesquisar":
		_, err = c.Empresas.Pesquisar(ctx, EmpresaFiltro{Q: o.s("q"), Uf: o.s("uf"), CodigoMunicipio: o.s("codigoMunicipio"),
			CnpjRaiz: o.s("cnpjRaiz"), Cnpj: o.s("cnpj"), DataAberturaInicio: o.s("dataAberturaInicio"),
			DataAberturaFim: o.s("dataAberturaFim"), CodigoSituacaoCadastral: o.s("codigoSituacaoCadastral"),
			Ceps: o.s("ceps"), Cnaes: o.s("cnaes"), Page: o.i("page"), PageSize: o.i("pageSize")})
	case "cnaes.listar":
		_, err = c.Cnaes.Listar(ctx, lista)
	case "estados.listar":
		_, err = c.Estados.Listar(ctx, lista)
	case "municipios.listar":
		_, err = c.Municipios.Listar(ctx, o.s("uf"), lista)
	case "municipios.obterPorId":
		_, err = c.Municipios.ObterPorID(ctx, num)
	case "mercado.obterCnae":
		_, err = c.Mercado.ObterCnae(ctx, str, &MercadoCnaeOptions{Uf: o.s("uf"), Meses: o.i("meses")})
	case "mercado.obterMunicipio":
		_, err = c.Mercado.ObterMunicipio(ctx, num)
	case "mercado.obterIndicador":
		_, err = c.Mercado.ObterIndicador(ctx, str, &IndicadorOptions{Inicio: o.s("inicio"), Fim: o.s("fim")})
	case "creditos.consultar":
		_, err = c.Creditos.Consultar(ctx)
	default:
		return fmt.Errorf("SDK sem %s", call)
	}
	return err
}

func canon(t *testing.T, raw string) string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestAppInfoIsSanitized(t *testing.T) {
	h := loadContract(t).Headers
	rt := &fakeRT{respond: func(int) (*http.Response, error) { return jsonResp(200, "{}", nil), nil }}
	if err := invoke(newTestClient(t, rt, WithAppInfo(h.AppInfo.Input)), "creditos.consultar", nil); err != nil {
		t.Fatal(err)
	}
	r := rt.seen[0].req
	info := r.Header.Get("X-NextApi-Client-Info")
	if !strings.HasSuffix(r.Header.Get("User-Agent"), " "+h.AppInfo.Sanitized) || !strings.HasSuffix(info, "; "+h.AppInfo.Sanitized) ||
		!regexp.MustCompile(h.Info).MatchString(info) {
		t.Fatalf("UA %q, info %q", r.Header.Get("User-Agent"), info)
	}
}

func TestContractCases(t *testing.T) {
	contract := loadContract(t)
	for _, c := range contract.Cases {
		t.Run(c.ID, func(t *testing.T) {
			rt := &fakeRT{respond: func(int) (*http.Response, error) { return jsonResp(200, string(c.Response), nil), nil }}
			call := c.Call
			if call == "" {
				call = c.ID
			}
			if err := invoke(newTestClient(t, rt), call, c.Args); err != nil {
				t.Fatal(err)
			}
			if len(rt.seen) != 1 {
				t.Fatalf("requisições: %d", len(rt.seen))
			}
			r := rt.seen[0]
			if r.req.Method != c.Method || r.req.URL.Scheme+"://"+r.req.URL.Host != DefaultBaseURL || r.req.URL.Path != c.Path {
				t.Fatalf("%s %s", r.req.Method, r.req.URL)
			}
			got := map[string]string{}
			for k, v := range r.req.URL.Query() {
				got[k] = v[0]
			}
			if fmt.Sprint(got) != fmt.Sprint(c.Query) {
				t.Fatalf("query %v, esperado %v", got, c.Query)
			}
			if r.req.Header.Get("Authorization") != "Bearer "+testKey {
				t.Fatal("Authorization")
			}
			if !regexp.MustCompile(`^nextapi-go/\d+\.\d+\.\d+ go/`).MatchString(r.req.Header.Get("User-Agent")) {
				t.Fatal("User-Agent: " + r.req.Header.Get("User-Agent"))
			}
			if !regexp.MustCompile(contract.Headers.Client).MatchString(r.req.Header.Get("X-NextApi-Client")) ||
				!regexp.MustCompile(contract.Headers.Info).MatchString(r.req.Header.Get("X-NextApi-Client-Info")) {
				t.Fatalf("cabeçalhos do cliente: %q %q", r.req.Header.Get("X-NextApi-Client"), r.req.Header.Get("X-NextApi-Client-Info"))
			}
			if len(c.Body) > 0 {
				if r.req.Header.Get("Content-Type") != "application/json" || canon(t, r.body) != canon(t, string(c.Body)) {
					t.Fatalf("body %s", r.body)
				}
			} else if r.body != "" {
				t.Fatalf("body inesperado %s", r.body)
			}
		})
	}
}

func TestInvalidInputNeverReachesTheAPI(t *testing.T) {
	for _, c := range loadContract(t).Invalid {
		t.Run(c.Reason, func(t *testing.T) {
			rt := &fakeRT{respond: func(int) (*http.Response, error) { return jsonResp(200, "{}", nil), nil }}
			err := invoke(newTestClient(t, rt), c.Call, c.Args)
			var e *Error
			if !errors.Is(err, ErrValidation) || !errors.As(err, &e) || e.Status != 0 {
				t.Fatalf("erro: %v", err)
			}
			if len(rt.seen) != 0 {
				t.Fatal("chegou à API")
			}
		})
	}
}

func TestValidInputs(t *testing.T) {
	for _, c := range loadContract(t).Valid {
		rt := &fakeRT{respond: func(int) (*http.Response, error) { return jsonResp(200, "{}", nil), nil }}
		if err := invoke(newTestClient(t, rt), c.Call, c.Args); err != nil {
			t.Fatal(err)
		}
		if rt.seen[0].req.URL.Path != c.Path {
			t.Fatal(rt.seen[0].req.URL.Path)
		}
	}
}

var sentinels = map[string]error{
	"authentication": ErrAuthentication, "insufficientCredits": ErrInsufficientCredits,
	"paidPlanRequired": ErrPaidPlanRequired, "notFound": ErrNotFound, "validation": ErrValidation,
	"rateLimit": ErrRateLimit, "server": ErrServer,
}

func TestErrors(t *testing.T) {
	for _, c := range loadContract(t).Errors {
		var body struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(c.Body, &body)
		t.Run(fmt.Sprintf("%d/%d", c.Status, body.Code), func(t *testing.T) {
			rt := &fakeRT{respond: func(int) (*http.Response, error) {
				return jsonResp(c.Status, string(c.Body), map[string]string{"X-Request-Id": "req-1", "Retry-After": "0"}), nil
			}}
			_, err := newTestClient(t, rt).Cnpj.Consultar(context.Background(), "11222333000181", nil)
			var e *Error
			if !errors.Is(err, sentinels[c.Error]) || !errors.As(err, &e) {
				t.Fatalf("%v não é %s", err, c.Error)
			}
			if e.Status != c.Status || e.Code != body.Code || e.Message != body.Message || e.RequestID != "req-1" {
				t.Fatalf("%+v", e)
			}
			want := 1
			if c.Retryable {
				want = 3
			}
			if len(rt.seen) != want {
				t.Fatalf("tentativas %d, esperado %d", len(rt.seen), want)
			}
		})
	}
}

func TestRetryThenSuccess(t *testing.T) {
	rt := &fakeRT{respond: func(n int) (*http.Response, error) {
		if n == 1 {
			return jsonResp(503, "{}", map[string]string{"Retry-After": "0"}), nil
		}
		return jsonResp(200, `{"cnpj":"1"}`, nil), nil
	}}
	r, err := newTestClient(t, rt).Cnpj.Consultar(context.Background(), "11222333000181", nil)
	if err != nil || *r.Cnpj != "1" || len(rt.seen) != 2 {
		t.Fatal(err, len(rt.seen))
	}
}

func TestBatchIsNeverRetried(t *testing.T) {
	rt := &fakeRT{respond: func(int) (*http.Response, error) {
		return jsonResp(503, `{"code":1014,"message":"x"}`, map[string]string{"Retry-After": "0"}), nil
	}}
	_, err := newTestClient(t, rt).Cnpj.ConsultarLote(context.Background(), []string{"11222333000181"})
	if !errors.Is(err, ErrServer) || len(rt.seen) != 1 {
		t.Fatal(err, len(rt.seen))
	}
}

func TestNetworkErrorRetriedWithoutKeyInMessage(t *testing.T) {
	rt := &fakeRT{respond: func(int) (*http.Response, error) { return nil, errors.New("connection refused") }}
	_, err := newTestClient(t, rt, WithMaxRetries(1)).Creditos.Consultar(context.Background())
	if !errors.Is(err, ErrConnection) || len(rt.seen) != 2 || strings.Contains(err.Error(), testKey) {
		t.Fatal(err, len(rt.seen))
	}
}

func TestRedirectIsRefused(t *testing.T) {
	rt := &fakeRT{respond: func(int) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://evil.example/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}}
	_, err := newTestClient(t, rt).Creditos.Consultar(context.Background())
	if !errors.Is(err, ErrConnection) || len(rt.seen) != 1 {
		t.Fatal(err, len(rt.seen))
	}
}

type hangRT struct{ calls int }

func (h *hangRT) RoundTrip(r *http.Request) (*http.Response, error) {
	h.calls++
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestTimeout(t *testing.T) {
	_, err := newTestClient(t, &hangRT{}, WithMaxRetries(0), WithTimeout(20*time.Millisecond)).Creditos.Consultar(context.Background())
	if !errors.Is(err, ErrConnection) || !strings.Contains(err.Error(), "20 ms") {
		t.Fatal(err)
	}
}

func TestCallerCancelStopsRetries(t *testing.T) {
	h := &hangRT{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, h).Creditos.Consultar(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || h.calls != 1 {
		t.Fatal(err, h.calls)
	}
}

func TestKeyIsNotShown(t *testing.T) {
	c := newTestClient(t, &fakeRT{})
	for _, s := range []string{fmt.Sprint(c), fmt.Sprintf("%v %+v %#v", c, c, c)} {
		if strings.Contains(s, testKey) {
			t.Fatal(s)
		}
	}
	if c.Environment != "sandbox" {
		t.Fatal(c.Environment)
	}
}

func TestBaseURLRequiresHTTPS(t *testing.T) {
	if _, err := NewClient(WithAPIKey(testKey), WithBaseURL("http://api.nextapi.com.br")); err == nil {
		t.Fatal("aceitou http")
	}
	if _, err := NewClient(WithAPIKey(testKey), WithBaseURL("http://localhost:5112")); err != nil {
		t.Fatal(err)
	}
}

func TestKeyFromEnvironment(t *testing.T) {
	t.Setenv("NEXTAPI_KEY", "")
	if _, err := NewClient(); !errors.Is(err, ErrAuthentication) {
		t.Fatal(err)
	}
	t.Setenv("NEXTAPI_KEY", "nxa_live_abc")
	c, err := NewClient()
	if err != nil || c.Environment != "production" {
		t.Fatal(err)
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	rt := &fakeRT{respond: func(int) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("<html>Bad Gateway</html>"))}, nil
	}}
	_, err := newTestClient(t, rt, WithMaxRetries(0)).Creditos.Consultar(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Kind != KindServer || e.Message != "HTTP 500" || string(e.Body) != "<html>Bad Gateway</html>" {
		t.Fatal(err)
	}
}

func TestAlphanumericCnpj(t *testing.T) {
	if v, _ := NormalizeCnpj("12.abc.345/01de-35"); v != "12ABC34501DE35" {
		t.Fatal(v)
	}
}

// ---- Fumaça contra a API publicada (sdks/smoke.sh): só roda com NEXTAPI_SMOKE_KEY. ----

func TestSmoke(t *testing.T) {
	key := os.Getenv("NEXTAPI_SMOKE_KEY")
	if key == "" {
		t.Skip("fumaça: defina NEXTAPI_SMOKE_KEY (sdks/smoke.sh)")
	}
	env := os.Getenv("NEXTAPI_SMOKE_ENV")
	if env == "" {
		env = "production"
	}
	data, err := os.ReadFile(contractPath("smoke.json"))
	if err != nil {
		t.Fatal(err)
	}
	var smoke struct {
		Steps []struct {
			ID     string            `json:"id"`
			Call   string            `json:"call"`
			Args   []json.RawMessage `json:"args"`
			Expect []string          `json:"expect"`
			Env    string            `json:"env"`
			BadKey bool              `json:"badKey"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &smoke); err != nil {
		t.Fatal(err)
	}
	for _, s := range smoke.Steps {
		if s.Env != "" && s.Env != env {
			continue
		}
		k := key
		if s.BadKey {
			k = key[:9] + "chave-invalida"
		}
		c, err := NewClient(WithAPIKey(k), WithMaxRetries(4))
		if err != nil {
			t.Fatal(err)
		}
		err = invoke(c, s.Call, s.Args)
		got, detail := "ok", ""
		var e *Error
		if errors.As(err, &e) {
			got = "other"
			if e.Kind == KindConnection {
				got = "connection"
			}
			for name, sentinel := range sentinels {
				if errors.Is(err, sentinel) {
					got = name
				}
			}
			detail = fmt.Sprintf(" (%d/%d %s req=%s)", e.Status, e.Code, e.Message, e.RequestID)
		} else if err != nil {
			got, detail = "other", " ("+err.Error()+")"
		}
		fmt.Printf("SMOKE go %s %s: %s%s\n", env, s.ID, got, detail)
		ok := false
		for _, x := range s.Expect {
			ok = ok || x == got
		}
		if !ok {
			t.Errorf("%s: veio %s, esperado %v", s.ID, got, s.Expect)
		}
	}
}
