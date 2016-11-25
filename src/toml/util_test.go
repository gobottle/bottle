package toml

import (
	"reflect"
	"testing"
)

var commonTestCasesForToSnakeCase = []struct {
	input, expect string
}{
	{"", ""},
	{"thequickbrownfoxjumpsoverthelazydog", "thequickbrownfoxjumpsoverthelazydog"},
	{"Thequickbrownfoxjumpsoverthelazydog", "thequickbrownfoxjumpsoverthelazydog"},
	{"ThequickbrownfoxjumpsoverthelazydoG", "thequickbrownfoxjumpsoverthelazydo_g"},
	{"TheQuickBrownFoxJumpsOverTheLazyDog", "the_quick_brown_fox_jumps_over_the_lazy_dog"},
	{"the_quick_brown_fox_jumps_over_the_lazy_dog", "the_quick_brown_fox_jumps_over_the_lazy_dog"},
	{"APIServer", "api_server"},
	{"WebUI", "web_ui"},
	{"API", "api"},
	{"ASCII", "ascii"},
	{"CPU", "cpu"},
	{"CSRF", "csrf"},
	{"CSS", "css"},
	{"DNS", "dns"},
	{"EOF", "eof"},
	{"GUID", "guid"},
	{"HTML", "html"},
	{"HTTP", "http"},
	{"HTTPS", "https"},
	{"ID", "id"},
	{"ip", "ip"},
	{"JSON", "json"},
	{"LHS", "lhs"},
	{"QPS", "qps"},
	{"RAM", "ram"},
	{"RHS", "rhs"},
	{"RPC", "rpc"},
	{"SLA", "sla"},
	{"SMTP", "smtp"},
	{"SQL", "sql"},
	{"SSH", "ssh"},
	{"TCP", "tcp"},
	{"TLS", "tls"},
	{"TTL", "ttl"},
	{"UDP", "udp"},
	{"UI", "ui"},
	{"UID", "uid"},
	{"UUID", "uuid"},
	{"URI", "uri"},
	{"URL", "url"},
	{"UTF8", "utf8"},
	{"VM", "vm"},
	{"XML", "xml"},
	{"XSRF", "xsrf"},
	{"XSS", "xss"},
}

func TestToSnakeCase(t *testing.T) {
	for _, v := range append(commonTestCasesForToSnakeCase, []struct {
		input, expect string
	}{
		{"ＴｈｅＱｕｉｃｋＢｒｏｗｎＦｏｘＯｖｅｒＴｈｅＬａｚｙＤｏｇ", "ｔｈｅ_ｑｕｉｃｋ_ｂｒｏｗｎ_ｆｏｘ_ｏｖｅｒ_ｔｈｅ_ｌａｚｙ_ｄｏｇ"},
	}...) {
		actual := toSnakeCase(v.input)
		expect := v.expect
		if !reflect.DeepEqual(actual, expect) {
			t.Errorf(`toSnakeCase(%#v) => %#v; want %#v`, v.input, actual, expect)
		}
	}
}
