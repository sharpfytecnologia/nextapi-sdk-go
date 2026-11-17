package nextapi

import (
	"context"
	"regexp"
)

// ConsultaOptions vale para Consultar e ObterSimples.
type ConsultaOptions struct {
	// ToleranceDays: idade máxima aceita do dado em cache, em dias (0 a 45). Int(0) força tempo real.
	ToleranceDays *int
}

// PaginaOptions vale para relacionamentos e empresas do sócio.
type PaginaOptions struct {
	Page, PageSize  int
	IncluirInativas *bool
}

func (o *PaginaOptions) query() string {
	if o == nil {
		return ""
	}
	return query{{"page", o.Page}, {"pageSize", o.PageSize}, {"incluirInativas", o.IncluirInativas}}.String()
}

// CnpjService: consultas por CNPJ, com ou sem pontuação, numérico ou alfanumérico.
type CnpjService struct{ c *Client }

func tolerance(o *ConsultaOptions) string {
	if o == nil {
		return ""
	}
	return query{{"toleranceDays", o.ToleranceDays}}.String()
}

// Consultar devolve os dados cadastrais do estabelecimento (1 crédito).
func (s *CnpjService) Consultar(ctx context.Context, cnpj string, opts *ConsultaOptions) (*CnpjItemResponse, error) {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return nil, err
	}
	return get[CnpjItemResponse](ctx, s.c, "/v1/cnpj/"+v+tolerance(opts))
}

// ConsultarAoVivo consulta em tempo real o provedor (ToleranceDays 0); exige plano que permita.
func (s *CnpjService) ConsultarAoVivo(ctx context.Context, cnpj string) (*CnpjItemResponse, error) {
	return s.Consultar(ctx, cnpj, &ConsultaOptions{ToleranceDays: Int(0)})
}

// ObterSimples devolve a situação no Simples Nacional e no MEI.
func (s *CnpjService) ObterSimples(ctx context.Context, cnpj string, opts *ConsultaOptions) (*CnpjSimplesResponse, error) {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return nil, err
	}
	return get[CnpjSimplesResponse](ctx, s.c, "/v1/cnpj/"+v+"/simples"+tolerance(opts))
}

// ObterInscricoesEstaduais devolve as inscrições estaduais, uma por UF.
func (s *CnpjService) ObterInscricoesEstaduais(ctx context.Context, cnpj string) ([]CnpjInscricaoEstadualResponse, error) {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return nil, err
	}
	return getList[CnpjInscricaoEstadualResponse](ctx, s.c, "/v1/cnpj/"+v+"/ie")
}

// ObterConformidade devolve sanções (CEIS, CNEP, CEPIM) e dívida ativa da União. Planos pagos ou créditos avulsos.
func (s *CnpjService) ObterConformidade(ctx context.Context, cnpj string) (*CnpjConformidadeResponse, error) {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return nil, err
	}
	return get[CnpjConformidadeResponse](ctx, s.c, "/v1/cnpj/"+v+"/conformidade")
}

// ObterRelacionamentos devolve sócios e empresas ligadas pelos mesmos sócios (grupo econômico).
func (s *CnpjService) ObterRelacionamentos(ctx context.Context, cnpj string, opts *PaginaOptions) (*CnpjRelacionamentoResponse, error) {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return nil, err
	}
	return get[CnpjRelacionamentoResponse](ctx, s.c, "/v1/cnpj/"+v+"/relacionamentos"+opts.query())
}

// ConsultarLote consulta até 100 CNPJs (1 crédito por CNPJ encontrado). Nunca é repetida automaticamente.
func (s *CnpjService) ConsultarLote(ctx context.Context, cnpjs []string) ([]CnpjItemResponse, error) {
	v, err := batch(cnpjs)
	if err != nil {
		return nil, err
	}
	return postList[CnpjItemResponse](ctx, s.c, "/v1/cnpj/batch", map[string]any{"cnpjs": v})
}

// ObterSimplesLote consulta o Simples Nacional de até 100 CNPJs. Nunca é repetida automaticamente.
func (s *CnpjService) ObterSimplesLote(ctx context.Context, cnpjs []string) ([]CnpjSimplesResponse, error) {
	v, err := batch(cnpjs)
	if err != nil {
		return nil, err
	}
	return postList[CnpjSimplesResponse](ctx, s.c, "/v1/cnpj/batch/simples", map[string]any{"cnpjs": v})
}

// SociosService: empresas por sócio.
type SociosService struct{ c *Client }

// ListarEmpresas devolve as empresas do sócio. O hash vem de ObterRelacionamentos (Socios[].HashSocio); com
// VinculoVerificavel falso ele é provisório: não guarde.
func (s *SociosService) ListarEmpresas(ctx context.Context, hashSocio string, opts *PaginaOptions) (*SocioEmpresasResponse, error) {
	v, err := normalizeHash(hashSocio)
	if err != nil {
		return nil, err
	}
	return get[SocioEmpresasResponse](ctx, s.c, "/v1/socios/"+v+"/empresas"+opts.query())
}

// CepsService: CEPs e geolocalização.
type CepsService struct{ c *Client }

// CepFiltro de Buscar. Q (texto livre) não combina com Logradouro.
type CepFiltro struct {
	Uf            string
	IdMunicipio   int
	NomeMunicipio string
	Logradouro    string
	Q             string
	Page          int
	PageSize      int
}

// GeoOptions de BuscarProximos e BuscarProximosDoCep.
type GeoOptions struct{ Page, PageSize int }

// Consultar devolve endereço e coordenadas de um CEP.
func (s *CepsService) Consultar(ctx context.Context, cep string) (*CepItemResponse, error) {
	v, err := NormalizeCep(cep)
	if err != nil {
		return nil, err
	}
	return get[CepItemResponse](ctx, s.c, "/v1/ceps/"+v)
}

// Buscar procura por UF, município, logradouro ou texto livre.
func (s *CepsService) Buscar(ctx context.Context, f CepFiltro) (*Page[CepItemResponse], error) {
	q := query{{"uf", f.Uf}, {"idMunicipio", f.IdMunicipio}, {"nomeMunicipio", f.NomeMunicipio},
		{"logradouro", f.Logradouro}, {"q", f.Q}, {"page", f.Page}, {"pageSize", f.PageSize}}
	return get[Page[CepItemResponse]](ctx, s.c, "/v1/ceps"+q.String())
}

// BuscarProximos devolve CEPs num raio (em metros) ao redor de uma coordenada.
func (s *CepsService) BuscarProximos(ctx context.Context, latitude, longitude, raioMetro float64, opts *GeoOptions) (*Page[CepItemResponse], error) {
	for _, err := range []error{requireRange(latitude, -90, 90, "latitude"), requireRange(longitude, -180, 180, "longitude"),
		requirePositive(raioMetro, "raioMetro")} {
		if err != nil {
			return nil, err
		}
	}
	if opts == nil {
		opts = &GeoOptions{}
	}
	q := query{{"latitude", latitude}, {"longitude", longitude}, {"raioMetro", raioMetro}, {"page", opts.Page}, {"pageSize", opts.PageSize}}
	return get[Page[CepItemResponse]](ctx, s.c, "/v1/ceps/geo"+q.String())
}

// BuscarProximosDoCep devolve CEPs num raio (em metros) ao redor de outro CEP.
func (s *CepsService) BuscarProximosDoCep(ctx context.Context, cep string, raioMetro float64, opts *GeoOptions) (*Page[CepItemResponse], error) {
	v, err := NormalizeCep(cep)
	if err != nil {
		return nil, err
	}
	if err := requirePositive(raioMetro, "raioMetro"); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &GeoOptions{}
	}
	q := query{{"raioMetro", raioMetro}, {"page", opts.Page}, {"pageSize", opts.PageSize}}
	return get[Page[CepItemResponse]](ctx, s.c, "/v1/ceps/geo/"+v+q.String())
}

// EmpresaFiltro de Pesquisar. Ceps e Cnaes aceitam vários valores separados por vírgula; datas em AAAA-MM-DD.
type EmpresaFiltro struct {
	Q, Uf, CodigoMunicipio, CnpjRaiz, Cnpj string
	DataAberturaInicio, DataAberturaFim    string
	CodigoSituacaoCadastral, Ceps, Cnaes   string
	Page, PageSize                         int
}

// EmpresasService: pesquisa de empresas.
type EmpresasService struct{ c *Client }

// Pesquisar procura empresas pelos filtros.
func (s *EmpresasService) Pesquisar(ctx context.Context, f EmpresaFiltro) (*Page[CnpjSimplifiedResponse], error) {
	q := query{{"q", f.Q}, {"uf", f.Uf}, {"codigoMunicipio", f.CodigoMunicipio}, {"cnpjRaiz", f.CnpjRaiz}, {"cnpj", f.Cnpj},
		{"dataAberturaInicio", f.DataAberturaInicio}, {"dataAberturaFim", f.DataAberturaFim},
		{"codigoSituacaoCadastral", f.CodigoSituacaoCadastral}, {"ceps", f.Ceps}, {"cnaes", f.Cnaes},
		{"page", f.Page}, {"pageSize", f.PageSize}}
	return get[Page[CnpjSimplifiedResponse]](ctx, s.c, "/v1/empresas"+q.String())
}

// ListaOptions de listagens paginadas.
type ListaOptions struct{ Page, PageSize int }

func (o *ListaOptions) q(extra ...struct {
	k string
	v any
}) string {
	if o == nil {
		o = &ListaOptions{}
	}
	return append(query(extra), query{{"page", o.Page}, {"pageSize", o.PageSize}}...).String()
}

// CnaesService: tabela CNAE.
type CnaesService struct{ c *Client }

func (s *CnaesService) Listar(ctx context.Context, opts *ListaOptions) (*Page[CnaeItemResponse], error) {
	return get[Page[CnaeItemResponse]](ctx, s.c, "/v1/cnaes"+opts.q())
}

// EstadosService: UFs.
type EstadosService struct{ c *Client }

func (s *EstadosService) Listar(ctx context.Context, opts *ListaOptions) (*Page[EstadoItemResponse], error) {
	return get[Page[EstadoItemResponse]](ctx, s.c, "/v1/estados"+opts.q())
}

// MunicipiosService: municípios.
type MunicipiosService struct{ c *Client }

// Listar devolve os municípios, de uma UF ou de todas (uf vazio).
func (s *MunicipiosService) Listar(ctx context.Context, uf string, opts *ListaOptions) (*Page[MunicipioItemResponse], error) {
	return get[Page[MunicipioItemResponse]](ctx, s.c, "/v1/municipios"+opts.q(struct {
		k string
		v any
	}{"uf", uf}))
}

// ObterPorID devolve o município pelo código da Receita, com a hierarquia do IBGE.
func (s *MunicipiosService) ObterPorID(ctx context.Context, id int) (*MunicipioDetalhadoResponse, error) {
	v, err := positiveID(id, "id")
	if err != nil {
		return nil, err
	}
	return get[MunicipioDetalhadoResponse](ctx, s.c, "/v1/municipios/"+v)
}

// MercadoService: dados de mercado agregados. Planos pagos ou créditos avulsos.
type MercadoService struct{ c *Client }

// MercadoCnaeOptions de ObterCnae.
type MercadoCnaeOptions struct {
	Uf    string
	Meses int
}

// IndicadorOptions de ObterIndicador; Inicio e Fim em AAAAMM.
type IndicadorOptions struct{ Inicio, Fim string }

var reCnaePunct = regexp.MustCompile(`[\s./-]`)

// ObterCnae devolve empresas ativas, aberturas e baixas de um CNAE (com ou sem pontuação).
func (s *MercadoService) ObterCnae(ctx context.Context, cnae string, opts *MercadoCnaeOptions) (*MercadoCnaeResponse, error) {
	v, err := pathSegment(reCnaePunct.ReplaceAllString(cnae, ""), "cnae")
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &MercadoCnaeOptions{}
	}
	return get[MercadoCnaeResponse](ctx, s.c, "/v1/mercado/cnaes/"+v+query{{"uf", opts.Uf}, {"meses", opts.Meses}}.String())
}

// ObterMunicipio devolve empresas e economia (PIB, população) de um município.
func (s *MercadoService) ObterMunicipio(ctx context.Context, id int) (*MercadoMunicipioResponse, error) {
	v, err := positiveID(id, "id")
	if err != nil {
		return nil, err
	}
	return get[MercadoMunicipioResponse](ctx, s.c, "/v1/mercado/municipios/"+v)
}

// ObterIndicador devolve a série de um indicador econômico (ex.: "ipca").
func (s *MercadoService) ObterIndicador(ctx context.Context, tipo string, opts *IndicadorOptions) (*MercadoIndicadorResponse, error) {
	v, err := pathSegment(tipo, "tipo")
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &IndicadorOptions{}
	}
	return get[MercadoIndicadorResponse](ctx, s.c, "/v1/mercado/indicadores/"+v+query{{"inicio", opts.Inicio}, {"fim", opts.Fim}}.String())
}

// CreditosService: saldo de créditos.
type CreditosService struct{ c *Client }

// Consultar devolve o saldo do plano e avulsos e os limites da chave. Não consome crédito.
func (s *CreditosService) Consultar(ctx context.Context) (*WorkspaceCreditsResponse, error) {
	return get[WorkspaceCreditsResponse](ctx, s.c, "/v1/credits")
}

func get[T any](ctx context.Context, c *Client, path string) (*T, error) {
	var out T
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func getList[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var out []T
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func postList[T any](ctx context.Context, c *Client, path string, body any) ([]T, error) {
	var out []T
	if err := c.post(ctx, path, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
