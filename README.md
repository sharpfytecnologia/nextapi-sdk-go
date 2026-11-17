# NextAPI para Go (`nextapi-sdk-go`)

SDK oficial da [NextAPI](https://nextapi.com.br) para Go 1.26+: CNPJ (inclusive alfanumérico), Simples Nacional, inscrições estaduais, conformidade, sócios, CEP com busca geográfica, pesquisa de empresas, localidades, mercado e créditos. Sem dependências: só a biblioteca padrão. Todo método recebe `context.Context`.

## Requisitos

- Go 1.26 ou mais recente (testado no 1.26 e no 1.27)
- Uma chave de API (veja abaixo)

## Instalação

```bash
go get github.com/sharpfytecnologia/nextapi-sdk-go
```

## Chave de API

Crie a sua em [app.nextapi.com.br](https://app.nextapi.com.br). São dois tipos:

| Prefixo | Ambiente | Custo |
|---|---|---|
| `nxa_test_…` | Sandbox: dados de um subconjunto real, limites próprios e CNPJs de teste que provocam cada erro | Nenhum: nada é debitado |
| `nxa_live_…` | Produção | Créditos do seu plano |

O ambiente sai do prefixo da chave; o endereço da API é o mesmo. Guarde a chave na variável de ambiente
`NEXTAPI_KEY`, que o cliente lê quando você não passa a chave. **Nunca** a coloque no código, no repositório ou num
front-end: quem tem a chave consome os seus créditos.

## Início rápido

```go
import nextapi "github.com/sharpfytecnologia/nextapi-sdk-go"

client, err := nextapi.NewClient() // lê NEXTAPI_KEY
if err != nil {
	log.Fatal(err)
}

empresa, err := client.Cnpj.Consultar(ctx, "11.222.333/0001-81", nil)
if err != nil {
	log.Fatal(err)
}
fmt.Println(*empresa.RazaoSocial)
```

Os campos das respostas são ponteiros: a API omite o que não tem, e `nil` distingue "ausente" de vazio.

## Configuração

```go
client, err := nextapi.NewClient(
	nextapi.WithAPIKey(os.Getenv("NEXTAPI_KEY")),      // padrão: NEXTAPI_KEY
	nextapi.WithBaseURL("https://api.nextapi.com.br"), // só https (http apenas para localhost)
	nextapi.WithTimeout(15*time.Second),               // por tentativa; padrão 30 s
	nextapi.WithMaxRetries(3),                         // padrão 2; 0 desliga
	nextapi.WithAppInfo("minha-app/1.4.0"),            // identifica a sua aplicação
	// nextapi.WithHTTPClient(&http.Client{...}),       // proxy, instrumentação, testes
)
```

| Opção | Padrão | Efeito |
|---|---|---|
| `WithAPIKey` | `NEXTAPI_KEY` | Chave de API; o prefixo define o ambiente (`client.Environment`) |
| `WithBaseURL` | `https://api.nextapi.com.br` | Endereço da API |
| `WithTimeout` | 30 s | Tempo limite de cada tentativa |
| `WithMaxRetries` | 2 | Novas tentativas em falha transitória, só em GET |
| `WithAppInfo` | — | Nome e versão da sua aplicação, nos cabeçalhos |
| `WithHTTPClient` | próprio, sem redirecionamento | `*http.Client` a usar; desligue o redirecionamento (`CheckRedirect`) |

O `ctx` de cada chamada cancela a requisição e as novas tentativas.

## Exemplos

### CNPJ

```go
// Cadastro (1 crédito). ToleranceDays: idade máxima aceita do dado em cache (0 a 45); nextapi.Int(0) força tempo real.
empresa, err := client.Cnpj.Consultar(ctx, "11222333000181", &nextapi.ConsultaOptions{ToleranceDays: nextapi.Int(30)})
aoVivo, err := client.Cnpj.ConsultarAoVivo(ctx, "11222333000181") // tempo real; exige plano que permita

simples, err := client.Cnpj.ObterSimples(ctx, "11222333000181", nil)
ies, err := client.Cnpj.ObterInscricoesEstaduais(ctx, "11222333000181")
conformidade, err := client.Cnpj.ObterConformidade(ctx, "11222333000181") // sanções e dívida ativa
alfanumerico, err := client.Cnpj.Consultar(ctx, "12.ABC.345/01DE-35", nil)
```

### Lotes (até 100 CNPJs)

```go
lote, err := client.Cnpj.ConsultarLote(ctx, []string{"11222333000181", "45997418000153"}) // 1 crédito por CNPJ encontrado
simplesLote, err := client.Cnpj.ObterSimplesLote(ctx, []string{"11222333000181"})

// Mais de 100: divida em blocos. Lote nunca é repetido sozinho; decida você numa falha.
for bloco := range slices.Chunk(todos, 100) {
	r, err := client.Cnpj.ConsultarLote(ctx, bloco)
	if err != nil {
		return err
	}
	resultados = append(resultados, r...)
}
```

### Sócios e grupo econômico

```go
rel, err := client.Cnpj.ObterRelacionamentos(ctx, "11222333000181", &nextapi.PaginaOptions{Page: 1, PageSize: 50})
for _, socio := range rel.Socios {
	// HashSocio só é estável quando VinculoVerificavel é true: não guarde os provisórios.
	empresas, err := client.Socios.ListarEmpresas(ctx, *socio.HashSocio, nil)
}
```

### CEP

```go
cep, err := client.Ceps.Consultar(ctx, "01001-000")
porRua, err := client.Ceps.Buscar(ctx, nextapi.CepFiltro{Uf: "SP", NomeMunicipio: "São Paulo", Logradouro: "Paulista"})
raio, err := client.Ceps.BuscarProximos(ctx, -23.5505, -46.6333, 500, nil)
vizinhos, err := client.Ceps.BuscarProximosDoCep(ctx, "01001000", 1000, &nextapi.GeoOptions{PageSize: 100})
```

### Pesquisa de empresas, com paginação

```go
filtro := nextapi.EmpresaFiltro{
	Uf:                 "SP",
	Cnaes:              "1091101", // vários: separados por vírgula
	DataAberturaInicio: "2024-01-01",
	PageSize:           100,
}
for filtro.Page = 1; ; filtro.Page++ {
	pagina, err := client.Empresas.Pesquisar(ctx, filtro)
	if err != nil {
		return err
	}
	for _, e := range pagina.Items {
		fmt.Println(*e.Cnpj, *e.RazaoSocial)
	}
	if int64(filtro.Page*pagina.PageSize) >= pagina.Total {
		break
	}
}
```

### Localidades, mercado e créditos

```go
estados, err := client.Estados.Listar(ctx, nil)
municipios, err := client.Municipios.Listar(ctx, "MG", &nextapi.ListaOptions{PageSize: 100})
bh, err := client.Municipios.ObterPorID(ctx, 3106200)
cnaes, err := client.Cnaes.Listar(ctx, &nextapi.ListaOptions{Page: 1, PageSize: 50})

mercado, err := client.Mercado.ObterCnae(ctx, "1091101", &nextapi.MercadoCnaeOptions{Uf: "SP", Meses: 12})
creditos, err := client.Creditos.Consultar(ctx)
```

## Erros

Toda falha vira uma exceção do SDK; a classe diz o que fazer, e o código detalha.

| Classe | Quando | Status / código | O que fazer |
|---|---|---|---|
| `ErrAuthentication` | Chave ausente, inválida ou revogada | 401 / 1002, 400 / 1001 | Confira a chave e o ambiente |
| `ErrInsufficientCredits` | Créditos do período acabaram | 405 / 1004, 1006 | Aguarde a renovação ou compre créditos avulsos |
| `ErrPaidPlanRequired` | Recurso exige plano pago | 405 / 1012, 1005 | Mude de plano |
| `ErrNotFound` | CNPJ, CEP ou município não existe | 404 / 1000 | Trate como resultado, não como falha |
| `ErrValidation` | Entrada recusada pela API ou pelo SDK | 400 / 1007, ou status 0 | Corrija a entrada; não repita |
| `ErrRateLimit` | Limite de requisições por minuto | 429 / 1003 | Já repetido sozinho; diminua o ritmo |
| `ErrServer` | Falha da API ou do provedor externo | 5xx / 1014, 9999 | 503 é repetido sozinho; persistindo, fale com o suporte |
| `ErrConnection` | Rede, tempo esgotado ou redirecionamento | status 0 | Verifique a conexão; GETs já foram repetidos |

Todas trazem o status HTTP, o código da API, a mensagem, o corpo da resposta e o **request id**. Ao falar com o
suporte, mande o request id: ele localiza a requisição nos nossos logs.

Todo erro do SDK é um `*nextapi.Error`; compare o tipo com `errors.Is` e leia os detalhes com `errors.As`:

```go
empresa, err := client.Cnpj.Consultar(ctx, cnpj, nil)
switch {
case err == nil:
	// ok
case errors.Is(err, nextapi.ErrNotFound):
	return nil // não existe
case errors.Is(err, nextapi.ErrInsufficientCredits):
	alertar("sem créditos")
case errors.Is(err, nextapi.ErrRateLimit):
	var e *nextapi.Error
	errors.As(err, &e)
	time.Sleep(e.RetryAfter)
default:
	var e *nextapi.Error
	if errors.As(err, &e) {
		log.Printf("NextAPI %d/%d %s req=%s", e.Status, e.Code, e.Message, e.RequestID)
	}
	return err
}
```

`Error` traz `Kind`, `Status`, `Code`, `Message`, `Body`, `RequestID`, `RetryAfter` e `Err` (a causa, em falha de
rede).

## Comportamento

**Novas tentativas.** Só requisições `GET` são repetidas, e só em falha transitória: `429`, `502`, `503`, `504` e
erro de rede. O cliente respeita o `Retry-After` da resposta (até 60 s); sem ele, espera de 0,5 s a 8 s, com
crescimento exponencial e variação aleatória. Os lotes (`POST`) **nunca** são repetidos sozinhos, porque consomem
créditos: numa falha, decida você se repete. O padrão é 2 novas tentativas; `WithMaxRetries(0)` desliga.

**Tempo limite.** `WithTimeout` por tentativa (padrão 30 s). Com novas tentativas, o tempo total pode passar disso.

**Validação local.** Entrada que a API recusaria com certeza nem sai do processo: vira
`ErrValidation` (status 0) antes da requisição. É o caso de CNPJ com tamanho ou dígitos verificadores errados,
lote vazio ou acima de 100, CEP sem 8 dígitos, raio menor ou igual a zero e latitude ou longitude fora da faixa.
CNPJ e CEP aceitam pontuação (`11.222.333/0001-81`, `01001-000`).

**CNPJ alfanumérico.** O formato novo da Receita (IN RFB 2.229/2024, a partir de julho de 2026) é aceito em todos os
métodos de CNPJ: `12.ABC.345/01DE-35` vira `12ABC34501DE35`, com letras em maiúsculas.

**Segurança.** A chave não aparece em logs, mensagens de erro nem na representação do cliente. O cliente não segue
redirecionamento, para a chave nunca ir para outro host, e só aceita `https` no `baseUrl` (`http` apenas para
`localhost`).

**Cabeçalhos enviados.** Além da autorização, cada requisição leva `User-Agent`, `X-NextApi-Client`
(`go/<versão>`) e `X-NextApi-Client-Info` (runtime, sistema e o seu `appInfo`). Servem para o suporte e para
medirmos o uso de cada versão; não carregam dado do seu negócio. O `appInfo` passa por um filtro: só
`A-Za-z0-9._/@+-`, até 64 caracteres.

**Reuso.** `*nextapi.Client` é seguro para uso concorrente: crie um e compartilhe entre goroutines.

## Sandbox

Com a chave `nxa_test_`, estes CNPJs provocam cada resposta da API, para você testar o tratamento de erro sem
gastar nada:

| CNPJ | Resposta |
|---|---|
| `99999991000125` | 404, não encontrado |
| `99999992000170` | 405 / 1004, sem créditos |
| `99999993000114` | 429 / 1003, limite por minuto |
| `99999994000169` | 503 / 1014, falha do provedor |
| `99999995000103` | sucesso depois de 8 s (consulta lenta) |
| `99999996000158` | 405 / 1012, exige plano pago |
| `99999997000100` | 401 / 1002, chave inválida |

O sandbox tem limite próprio de requisições por minuto e por dia, e os dados vêm de um subconjunto real: um CNPJ ou
CEP pode existir em produção e não no sandbox.

## Testes

Os testes não usam rede: conferem cada método contra o contrato da API, em `contract/cases.json` (rota, parâmetros, corpo, respostas e erros).

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./...
```

Com `NEXTAPI_SMOKE_KEY` definida (e `NEXTAPI_SMOKE_ENV=sandbox` ou `production`), roda também a fumaça contra a API
publicada, com os passos de `contract/smoke.json`.

## Suporte e licença

Dúvidas e problemas: abra uma issue neste repositório ou escreva pelo [app.nextapi.com.br](https://app.nextapi.com.br),
com o request id do erro. Licença MIT.
