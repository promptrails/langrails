// Package dashscope provides an Alibaba DashScope (Qwen) LLM provider for langrails.
//
// It uses the OpenAI-compatible mode of DashScope and is a thin wrapper around the compat package.
// The default base URL targets the international (Singapore) endpoint; switch to
// https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions via WithBaseURL for the
// mainland China endpoint.
package dashscope
