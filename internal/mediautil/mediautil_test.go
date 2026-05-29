package mediautil

import "testing"

func TestParseImageURL_DataURI(t *testing.T) {
	mt, data, url, isB64 := ParseImageURL("data:image/png;base64,AAAB")
	if !isB64 {
		t.Fatal("expected base64")
	}
	if mt != "image/png" || data != "AAAB" || url != "" {
		t.Errorf("got mt=%q data=%q url=%q", mt, data, url)
	}
}

func TestParseImageURL_HTTP(t *testing.T) {
	mt, data, url, isB64 := ParseImageURL("https://example.com/cat.jpg")
	if isB64 {
		t.Fatal("expected non-base64")
	}
	if url != "https://example.com/cat.jpg" || mt != "" || data != "" {
		t.Errorf("got mt=%q data=%q url=%q", mt, data, url)
	}
}

func TestParseImageURL_NonBase64DataURI(t *testing.T) {
	_, _, url, isB64 := ParseImageURL("data:text/plain,hello")
	if isB64 {
		t.Error("non-base64 data URI should not be treated as base64")
	}
	if url != "data:text/plain,hello" {
		t.Errorf("url = %q", url)
	}
}

func TestImageFormat(t *testing.T) {
	if got := ImageFormat("image/jpeg"); got != "jpeg" {
		t.Errorf("got %q", got)
	}
	if got := ImageFormat("application/pdf"); got != "" {
		t.Errorf("expected empty for non-image, got %q", got)
	}
}
