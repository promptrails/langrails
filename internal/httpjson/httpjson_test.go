package httpjson

import "testing"

func TestDetailError(t *testing.T) {
	tests := map[string]string{
		`{"detail": "bad model"}`:                  "bad model",
		`{"detail": [{"msg": "a"}, {"msg": "b"}]}`: "a; b",
		`{"detail": {"code": 1}}`:                  `{"code": 1}`,
		`{"error": {"message": "openai style"}}`:   "openai style",
		`not json`:                                 "",
	}
	for body, want := range tests {
		if got := DetailError([]byte(body)); got != want {
			t.Errorf("DetailError(%s) = %q, want %q", body, got, want)
		}
	}
}
