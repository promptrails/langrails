package langrails

// ToolChoiceMode controls whether and which tool the model calls.
type ToolChoiceMode string

const (
	// ToolChoiceAuto lets the model decide whether to call a tool (provider default).
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceNone forbids tool calls; the model must answer with text.
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceRequired forces the model to call at least one tool.
	ToolChoiceRequired ToolChoiceMode = "required"
	// ToolChoiceTool forces the model to call the specific tool named in ToolChoice.Name.
	ToolChoiceTool ToolChoiceMode = "tool"
)

// ToolChoice controls tool-calling behavior for a request.
type ToolChoice struct {
	// Mode is the tool-choice strategy.
	Mode ToolChoiceMode
	// Name is the tool to force when Mode is ToolChoiceTool.
	Name string
}

// AutoToolChoice returns a ToolChoice that lets the model decide.
func AutoToolChoice() *ToolChoice { return &ToolChoice{Mode: ToolChoiceAuto} }

// NoToolChoice returns a ToolChoice that forbids tool calls.
func NoToolChoice() *ToolChoice { return &ToolChoice{Mode: ToolChoiceNone} }

// RequiredToolChoice returns a ToolChoice that forces some tool call.
func RequiredToolChoice() *ToolChoice { return &ToolChoice{Mode: ToolChoiceRequired} }

// ForceTool returns a ToolChoice that forces the named tool.
func ForceTool(name string) *ToolChoice { return &ToolChoice{Mode: ToolChoiceTool, Name: name} }
