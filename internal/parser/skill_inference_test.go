package parser

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/testjsonl"
)

func TestInferCursorSkillNameFromReadFile(t *testing.T) {
	path := writeTestSkill(t, "foo", "foo")

	_, _, toolCalls := extractAssistantContent([]string{
		"[Tool call] ReadFile",
		"  path=" + path,
	})

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "ReadFile", toolCalls[0].ToolName)
	assert.Equal(t, "foo", toolCalls[0].SkillName)
}

func TestInferCursorSkillNameUsesFrontmatterName(t *testing.T) {
	path := writeTestSkill(t, "index", "data-analytics:index")

	_, _, toolCalls := extractAssistantContent([]string{
		"[Tool call] ReadFile",
		`  {"path":"` + path + `"}`,
	})

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "data-analytics:index", toolCalls[0].SkillName)
}

func TestInferCursorSkillNameIgnoresDiscoveryAndNonSkillPaths(t *testing.T) {
	path := writeTestSkill(t, "foo", "foo")

	tests := []struct {
		name string
		line string
	}{
		{
			name: "glob discovery",
			line: `[Tool call] Glob
  {"target_directory":"` + filepath.Dir(path) + `","glob_pattern":"**/SKILL.md"}`,
		},
		{
			name: "non skill path",
			line: `[Tool call] ReadFile
  path=` + filepath.Join(t.TempDir(), "README.md"),
		},
		{
			name: "empty input",
			line: "[Tool call] ReadFile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, toolCalls := extractAssistantContent(
				splitTestLines(tt.line),
			)
			require.Len(t, toolCalls, 1)
			assert.Empty(t, toolCalls[0].SkillName)
		})
	}
}

func TestInferCodexSkillNameFromReadCommands(t *testing.T) {
	path := writeTestSkill(t, "foo", "foo")

	for _, cmd := range []string{
		"cat " + path,
		"sed -n '1,220p' " + path,
		"head -40 " + path,
		"tail -40 " + path,
		"rg name " + path,
		"grep name " + path,
		"cd /tmp && sed -n '1,220p' " + path,
	} {
		t.Run(cmd, func(t *testing.T) {
			got := inferCodexSkillName(
				"exec_command",
				`{"cmd":`+quoteJSON(t, cmd)+`}`,
			)
			assert.Equal(t, "foo", got)
		})
	}
}

func TestInferCodexSkillNameIgnoresWriteCommands(t *testing.T) {
	path := writeTestSkill(t, "foo", "foo")

	for _, cmd := range []string{
		"cp " + path + " /tmp/SKILL.md",
		"mv " + path + " /tmp/SKILL.md",
		"mkdir -p " + filepath.Dir(path),
		"git add " + path,
		"sed -i '' 's/a/b/' " + path,
		"cat > " + path,
	} {
		t.Run(cmd, func(t *testing.T) {
			got := inferCodexSkillName(
				"exec_command",
				`{"cmd":`+quoteJSON(t, cmd)+`}`,
			)
			assert.Empty(t, got)
		})
	}
}

func TestParseCodexSessionInfersSkillName(t *testing.T) {
	path := writeTestSkill(t, "index", "data-analytics:index")
	content := testjsonl.JoinJSONL(
		testjsonl.CodexSessionMetaJSON("skill-read", "/tmp", "user", tsEarly),
		testjsonl.CodexMsgJSON("user", "use the dashboard skill", tsEarlyS1),
		testjsonl.CodexFunctionCallArgsJSON("exec_command", map[string]any{
			"cmd": "sed -n '1,220p' '" + path + "'",
		}, tsEarlyS5),
	)

	_, msgs := runCodexParserTest(t, "skill-read.jsonl", content, false)

	require.Len(t, msgs, 2)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "data-analytics:index", msgs[1].ToolCalls[0].SkillName)
}

func writeTestSkill(t *testing.T, folder, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "skills", folder)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "SKILL.md")
	content := "---\nname: " + name + "\ndescription: Test skill\n---\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func splitTestLines(s string) []string {
	return strings.Split(s, "\n")
}

func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	return strconv.Quote(s)
}
