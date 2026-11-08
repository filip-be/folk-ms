package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

// langCookie holds the admin UI language the user chose (pl or en). The admin
// SPA sets it, so it is not HttpOnly; without it the browser's languages
// decide. The SPA applies the same rule to navigator.languages.
const langCookie = "folkms-lang"

// requestLang returns the UI language of a request: the langCookie, else the
// first of Polish or English in Accept-Language by preference, else English.
func requestLang(r *http.Request) domain.Lang {
	if c, err := r.Cookie(langCookie); err == nil {
		switch domain.Lang(c.Value) {
		case domain.PL, domain.EN:
			return domain.Lang(c.Value)
		}
	}
	return acceptLang(r.Header.Get("Accept-Language"))
}

// acceptLang picks pl or en from an Accept-Language header (RFC 9110 §12.5.4).
func acceptLang(h string) domain.Lang {
	best, bestQ := domain.EN, 0.0
	for _, part := range strings.Split(h, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			q = f
		}
		primary, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
		var l domain.Lang
		switch primary {
		case "pl":
			l = domain.PL
		case "en":
			l = domain.EN
		default:
			continue
		}
		if q > bestQ {
			best, bestQ = l, q
		}
	}
	return best
}

// msg is a bilingual message.
func msg(pl, en string) domain.Text { return domain.Text{PL: pl, EN: en} }
