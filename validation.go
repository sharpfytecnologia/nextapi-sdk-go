package nextapi

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	reCnpj      = regexp.MustCompile(`^[0-9A-Z]{12}[0-9]{2}$`)
	reCep       = regexp.MustCompile(`^[0-9]{8}$`)
	reHash      = regexp.MustCompile(`^[0-9a-fA-F]{20}$`)
	reSegment   = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)
	reCnpjPunct = regexp.MustCompile(`[\s./-]`)
	reCepPunct  = regexp.MustCompile(`[\s.-]`)
)

// NormalizeCnpj tira a pontuação e põe em maiúsculas. Aceita o CNPJ alfanumérico (12 alfanuméricos e 2 dígitos).
func NormalizeCnpj(cnpj string) (string, error) {
	v := strings.ToUpper(reCnpjPunct.ReplaceAllString(cnpj, ""))
	if !reCnpj.MatchString(v) {
		return "", invalid("CNPJ inválido: %q. Informe 14 caracteres, com ou sem pontuação.", cnpj)
	}
	return v, nil
}

// NormalizeCep aceita "01001-000" e devolve os 8 dígitos.
func NormalizeCep(cep string) (string, error) {
	v := reCepPunct.ReplaceAllString(cep, "")
	if !reCep.MatchString(v) {
		return "", invalid("CEP inválido: %q. Informe 8 dígitos.", cep)
	}
	return v, nil
}

func normalizeHash(h string) (string, error) {
	v := strings.TrimSpace(h)
	if !reHash.MatchString(v) {
		return "", invalid("hashSocio inválido: são 20 caracteres hexadecimais.")
	}
	return strings.ToLower(v), nil
}

func pathSegment(v, name string) (string, error) {
	t := strings.TrimSpace(v)
	if !reSegment.MatchString(t) {
		return "", invalid("%s inválido: %q.", name, v)
	}
	return url.PathEscape(t), nil
}

func positiveID(id int, name string) (string, error) {
	if id <= 0 {
		return "", invalid("%s inválido: %d.", name, id)
	}
	return strconv.Itoa(id), nil
}

func requirePositive(v float64, name string) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return invalid("%s tem de ser maior que zero.", name)
	}
	return nil
}

func requireRange(v, lo, hi float64, name string) error {
	if math.IsNaN(v) || v < lo || v > hi {
		return invalid("%s tem de estar entre %g e %g.", name, lo, hi)
	}
	return nil
}

func batch(cnpjs []string) ([]string, error) {
	if len(cnpjs) == 0 {
		return nil, invalid("Informe ao menos um CNPJ.")
	}
	if len(cnpjs) > 100 {
		return nil, invalid("O lote aceita até 100 CNPJs; vieram %d.", len(cnpjs))
	}
	out := make([]string, len(cnpjs))
	for i, c := range cnpjs {
		v, err := NormalizeCnpj(c)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// query monta a query string na ordem informada, pulando valores ausentes (nil, "" ou 0 em inteiros opcionais).
type query []struct {
	k string
	v any
}

func (q query) String() string {
	var b strings.Builder
	for _, p := range q {
		var s string
		switch v := p.v.(type) {
		case nil:
			continue
		case string:
			s = v
		case int:
			if v == 0 {
				continue
			}
			s = strconv.Itoa(v)
		case *int:
			if v == nil {
				continue
			}
			s = strconv.Itoa(*v)
		case *bool:
			if v == nil {
				continue
			}
			s = strconv.FormatBool(*v)
		case float64:
			s = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			s = fmt.Sprint(v)
		}
		if s == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteByte('?')
		} else {
			b.WriteByte('&')
		}
		b.WriteString(url.QueryEscape(p.k) + "=" + url.QueryEscape(s))
	}
	return b.String()
}

// Int devolve um ponteiro para v, para os parâmetros em que 0 tem significado (ex.: ToleranceDays).
func Int(v int) *int { return &v }

// Bool devolve um ponteiro para v.
func Bool(v bool) *bool { return &v }
