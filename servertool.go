package langrails

// ServerToolType identifies a provider-hosted tool — one the provider executes
// itself (server-side), as opposed to ToolDefinition functions that the caller
// executes. The set is intentionally extensible (web search today; code
// execution, file search, etc. later).
type ServerToolType string

const (
	// ServerToolWebSearch enables the provider's built-in web search/grounding.
	ServerToolWebSearch ServerToolType = "web_search"
)

// ServerTool enables a provider-hosted tool for a request. Set the option field
// matching Type; unsupported tools are ignored by a given provider.
type ServerTool struct {
	// Type selects which server-hosted tool to enable.
	Type ServerToolType

	// WebSearch holds web-search options when Type is ServerToolWebSearch.
	// May be nil to use provider defaults.
	WebSearch *WebSearchOptions
}

// WebSearchOptions configures the built-in web search server tool. All fields
// are optional; zero values use the provider's defaults, and options not
// supported by a given provider are ignored.
type WebSearchOptions struct {
	// MaxUses caps how many searches the model may run for this request.
	MaxUses int
	// AllowedDomains restricts results to these domains, when supported.
	AllowedDomains []string
	// BlockedDomains excludes these domains from results, when supported.
	BlockedDomains []string
	// UserLocation is an optional location hint (e.g. "US") for localized results.
	UserLocation string
}

// WebSearch returns a ServerTool that enables built-in web search with the
// given options (pass nil for provider defaults).
func WebSearch(opts *WebSearchOptions) ServerTool {
	return ServerTool{Type: ServerToolWebSearch, WebSearch: opts}
}

// Citation is a source returned by provider-native web search or grounding.
type Citation struct {
	// URL is the source URL.
	URL string
	// Title is the source title, when provided.
	Title string
	// Snippet is an optional excerpt from the source.
	Snippet string
	// StartIndex and EndIndex are the span in CompletionResponse.Content that
	// this citation supports, when the provider supplies character offsets.
	// Both are 0 when unavailable.
	StartIndex int
	EndIndex   int
}
