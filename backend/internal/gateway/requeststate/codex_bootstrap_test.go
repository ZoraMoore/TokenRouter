package requeststate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	automationBootstrapPrompt = "Review the project and report any important changes."
	delegationEnvelope        = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>do the work</input></codex_delegation>`
	heartbeatEnvelope         = `<heartbeat><automation_id>wiki</automation_id></heartbeat>`
)

// TestCodexEnvelopeValidators 覆盖跨任务消息、自动化启动提示和心跳的信封格式判断。
func TestCodexEnvelopeValidators(t *testing.T) {
	require.True(t, ValidCodexDelegationEnvelope(delegationEnvelope))
	for _, envelope := range []string{
		`<codex_delegation><source_thread_id>thread-1</source_thread_id></codex_delegation>`,
		`<codex_delegation><source_thread_id></source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input> </input></codex_delegation>`,
		`prefix<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>suffix`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input>work</input><extra>x</extra></codex_delegation>`,
		`<codex_delegation><source_thread_id>thread-1</source_thread_id><input><nested>work</nested></input></codex_delegation>`,
		`<codex_delegation xmlns="urn:test"><source_thread_id>thread-1</source_thread_id><input>work</input></codex_delegation>`,
	} {
		require.False(t, ValidCodexDelegationEnvelope(envelope), envelope)
	}

	require.True(t, ValidCodexAutomationHeartbeat(heartbeatEnvelope))
	for _, output := range []string{
		`<result><automation_id>wiki</automation_id></result>`,
		`<heartbeat status="ok"><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat><automation_id>wiki</automation_id><status>ok</status></heartbeat>`,
		`<heartbeat><automation_id> wiki </automation_id></heartbeat>`,
		`<heartbeat><automation_id>../wiki</automation_id></heartbeat>`,
		`<heartbeat><!-- ok --><automation_id>wiki</automation_id></heartbeat>`,
		`<heartbeat><automation_id>wiki</automation_id></heartbeat>extra`,
	} {
		require.False(t, ValidCodexAutomationHeartbeat(output), output)
	}

	valid := codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt)
	require.True(t, ValidCodexAutomationBootstrap(valid))
	for _, output := range []string{
		strings.Replace(valid, "/wiki/memory.md", "/other/memory.md", 1),
		codexAutomationBootstrap("../wiki", "never", automationBootstrapPrompt),
		codexAutomationBootstrap("wiki", "yesterday", automationBootstrapPrompt),
		codexAutomationBootstrap("wiki", "2026-09-01T02:06:34.536Z (1)", automationBootstrapPrompt),
		strings.Replace(valid, "\n\n"+automationBootstrapPrompt, "\n"+automationBootstrapPrompt, 1),
		codexAutomationBootstrap("wiki", "never", " \n"),
	} {
		require.False(t, ValidCodexAutomationBootstrap(output), output)
	}
}

func codexAutomationBootstrap(automationID, lastRun, prompt string) string {
	return "Automation: Scheduled project review\n" +
		"Automation ID: " + automationID + "\n" +
		"Automation memory: $CODEX_HOME/automations/" + automationID + "/memory.md\n" +
		"Last run: " + lastRun + "\n\n" + prompt
}

func codexAutomationBootstrapBody(t *testing.T, output, callID string) []byte {
	t.Helper()
	return []byte(`{"model":"gpt-5","input":[{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` +
		mustJSON(t, output) + callID + `}]}`)
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
