package nextapi

import "context"

// MonitoramentosService: monitoramento de empresas, a carteira de CNPJs acompanhados e os eventos de mudança deles.
// Cada CNPJ custa créditos por mês, debitados na inclusão; a chave de teste (nxa_test_) não debita.
type MonitoramentosService struct{ c *Client }

// IncluirOptions são as preferências de Incluir; valem para todos os CNPJs da chamada.
type IncluirOptions struct {
	// Eventos: tipos de evento (cadastro.alterado, socios.alterado). Nil: todos os disponíveis.
	Eventos []string
	// Cadencia: "mensal" (padrão).
	Cadencia string
	// Canais: avisos além do polling, "email" e/ou "webhook". Nil: nenhum.
	Canais []string
}

// EventosOptions filtra e pagina ListarEventos.
type EventosOptions struct {
	// Cursor: ProximoCursor da página anterior.
	Cursor string
	// Tipo: tipos de evento separados por vírgula.
	Tipo string
	// Desde: só eventos observados a partir deste instante (ISO 8601, UTC).
	Desde string
	// Limite: eventos por página (padrão e máximo 100).
	Limite int
}

// Incluir põe até 100 CNPJs na carteira; o resultado vem CNPJ a CNPJ. Sem saldo para todos, devolve erro
// ErrInsufficientCredits e não inclui nenhum. Nunca é repetida automaticamente (debita créditos).
func (s *MonitoramentosService) Incluir(ctx context.Context, cnpjs []string, opts *IncluirOptions) (*MonitoramentoInscricaoResponse, error) {
	v, err := batch(cnpjs)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"cnpjs": v}
	if opts != nil {
		if opts.Eventos != nil {
			body["eventos"] = opts.Eventos
		}
		if opts.Cadencia != "" {
			body["cadencia"] = opts.Cadencia
		}
		if opts.Canais != nil {
			body["canais"] = opts.Canais
		}
	}
	var out MonitoramentoInscricaoResponse
	if err := s.c.post(ctx, "/v1/monitoramentos", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Listar devolve os CNPJs ativos na carteira, em ordem de CNPJ, paginados.
func (s *MonitoramentosService) Listar(ctx context.Context, opts *ListaOptions) (*Page[MonitoramentoItemResponse], error) {
	return get[Page[MonitoramentoItemResponse]](ctx, s.c, "/v1/monitoramentos"+opts.q())
}

// Remover tira o CNPJ da carteira. Os eventos já observados continuam no polling.
func (s *MonitoramentosService) Remover(ctx context.Context, cnpj string) error {
	v, err := NormalizeCnpj(cnpj)
	if err != nil {
		return err
	}
	return s.c.delete(ctx, "/v1/monitoramentos/"+v)
}

// ListarEventos devolve os eventos da carteira, em ordem. Guarde ProximoCursor e envie em Cursor na chamada seguinte.
// Entrega pelo menos uma vez: deduplique pelo Id.
func (s *MonitoramentosService) ListarEventos(ctx context.Context, opts *EventosOptions) (*MonitoramentoEventosResponse, error) {
	if opts == nil {
		opts = &EventosOptions{}
	}
	return get[MonitoramentoEventosResponse](ctx, s.c, "/v1/monitoramentos/eventos"+
		query{{"cursor", opts.Cursor}, {"tipo", opts.Tipo}, {"desde", opts.Desde}, {"limite", opts.Limite}}.String())
}
