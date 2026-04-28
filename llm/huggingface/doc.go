// Package huggingface provides a Hugging Face Inference Providers (Router) LLM provider for langrails.
//
// It uses the OpenAI-compatible router endpoint and is a thin wrapper around the compat package.
// Models are addressed as "<owner>/<model>", optionally suffixed with a provider policy
// (":fastest", ":cheapest") or a specific provider (":sambanova", ":together", ...).
package huggingface
