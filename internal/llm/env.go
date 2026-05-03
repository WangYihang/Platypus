package llm

import "os"

// FromEnv builds a Client from the three documented env vars. The
// returned client may be nil-Available (no API key set) — callers
// should check c.Available() before dialling. The function NEVER
// returns nil so callers don't need to nil-check the receiver.
//
//   PLATYPUS_LLM_BASE_URL — defaults to OpenAI's /v1
//   PLATYPUS_LLM_API_KEY  — required to make calls; absent → disabled
//   PLATYPUS_LLM_MODEL    — defaults to gpt-4o-mini
func FromEnv() *Client {
	return New(
		os.Getenv("PLATYPUS_LLM_BASE_URL"),
		os.Getenv("PLATYPUS_LLM_API_KEY"),
		os.Getenv("PLATYPUS_LLM_MODEL"),
	)
}
