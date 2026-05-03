package agent

// flattenEnv turns a map into the KEY=VALUE slice os/exec.Cmd.Env
// expects. The legacy agent.HandleExec used this; it now survives
// only because the streaming PTY path
// (HandleProcessStream → process_stream.go) takes the same input
// shape.
func flattenEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
