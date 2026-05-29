package gemini

import "encoding/json"

// Request types

type request struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
	Tools             []toolDeclaration `json:"tools,omitempty"`
	ToolConfig        *toolConfig       `json:"toolConfig,omitempty"`
}

type toolConfig struct {
	FunctionCallingConfig functionCallingConfig `json:"functionCallingConfig"`
}

type functionCallingConfig struct {
	Mode                 string   `json:"mode"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FileData         *fileData         `json:"fileData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"` // base64
}

type fileData struct {
	MIMEType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type functionCall struct {
	Name             string                 `json:"name"`
	Args             map[string]interface{} `json:"args"`
	ThoughtSignature string                 `json:"thoughtSignature,omitempty"`
}

type functionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type generationConfig struct {
	Temperature      *float64         `json:"temperature,omitempty"`
	MaxTokens        *int             `json:"maxOutputTokens,omitempty"`
	TopP             *float64         `json:"topP,omitempty"`
	TopK             *int             `json:"topK,omitempty"`
	StopSequences    []string         `json:"stopSequences,omitempty"`
	ResponseMIMEType string           `json:"responseMimeType,omitempty"`
	ResponseSchema   *json.RawMessage `json:"responseSchema,omitempty"`
	ThinkingConfig   *thinkingConfig  `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type toolDeclaration struct {
	FunctionDeclarations []functionDecl `json:"functionDeclarations,omitempty"`
	GoogleSearch         *struct{}      `json:"googleSearch,omitempty"`
}

type functionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Response types

type response struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata,omitempty"`
}

type candidate struct {
	Content           content            `json:"content"`
	FinishReason      string             `json:"finishReason"`
	GroundingMetadata *groundingMetadata `json:"groundingMetadata,omitempty"`
}

type groundingMetadata struct {
	GroundingChunks []struct {
		Web *struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		} `json:"web,omitempty"`
	} `json:"groundingChunks,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
}

// Error response

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Status  string `json:"status"`
		Code    int    `json:"code"`
	} `json:"error"`
}
