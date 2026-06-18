package langrails

import "testing"

func TestWebSearch_NilOptions(t *testing.T) {
	st := WebSearch(nil)
	if st.Type != ServerToolWebSearch {
		t.Errorf("Type = %q, want %q", st.Type, ServerToolWebSearch)
	}
	if st.WebSearch != nil {
		t.Errorf("WebSearch = %+v, want nil (provider defaults)", st.WebSearch)
	}
}

func TestWebSearch_WithOptions(t *testing.T) {
	opts := &WebSearchOptions{
		MaxUses:        3,
		AllowedDomains: []string{"example.com"},
		BlockedDomains: []string{"spam.com"},
		UserLocation:   "US",
	}
	st := WebSearch(opts)

	if st.Type != ServerToolWebSearch {
		t.Errorf("Type = %q, want %q", st.Type, ServerToolWebSearch)
	}
	if st.WebSearch == nil {
		t.Fatal("WebSearch options should be preserved")
	}
	if st.WebSearch.MaxUses != 3 {
		t.Errorf("MaxUses = %d, want 3", st.WebSearch.MaxUses)
	}
	if len(st.WebSearch.AllowedDomains) != 1 || st.WebSearch.AllowedDomains[0] != "example.com" {
		t.Errorf("AllowedDomains = %v", st.WebSearch.AllowedDomains)
	}
	if len(st.WebSearch.BlockedDomains) != 1 || st.WebSearch.BlockedDomains[0] != "spam.com" {
		t.Errorf("BlockedDomains = %v", st.WebSearch.BlockedDomains)
	}
	if st.WebSearch.UserLocation != "US" {
		t.Errorf("UserLocation = %q, want %q", st.WebSearch.UserLocation, "US")
	}
}

func TestWebSearch_PreservesOptionPointer(t *testing.T) {
	opts := &WebSearchOptions{MaxUses: 1}
	st := WebSearch(opts)
	if st.WebSearch != opts {
		t.Error("WebSearch should carry the caller's options pointer unchanged")
	}
}
