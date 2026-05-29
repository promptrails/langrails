// Package bedrock provides an Amazon Bedrock LLM provider for langrails.
//
// It uses Bedrock's unified Converse API, so a single implementation works
// across model families (Anthropic Claude, Meta Llama, Amazon Nova/Titan,
// Mistral, Cohere, and others). Requests are signed with AWS Signature V4 using
// only the standard library, keeping langrails dependency-free.
//
// Credentials and region are read from the standard AWS environment variables
// (AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN) by
// default, or supplied explicitly:
//
//	p := bedrock.New(
//		bedrock.WithRegion("us-east-1"),
//		bedrock.WithStaticCredentials(id, secret, ""),
//	)
//	resp, err := p.Complete(ctx, &langrails.CompletionRequest{
//		Model:    "anthropic.claude-3-5-sonnet-20241022-v2:0",
//		Messages: []langrails.Message{{Role: "user", Content: "Hello"}},
//	})
//
// The CompletionRequest.Model field is a Bedrock model or inference-profile ID.
package bedrock
