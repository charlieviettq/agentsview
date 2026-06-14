package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
)

var (
	skillNameByPath sync.Map
	skillPathRE     = regexp.MustCompile(`(?:"([^"]*[/\\]SKILL\.md)"|'([^']*[/\\]SKILL\.md)'|(\S*[/\\]SKILL\.md))`)
	readCommandRE   = regexp.MustCompile(`(?:^|(?:&&|\|\||[;&|])\s*)(?:[A-Za-z0-9_.-]+/)?(?:cat|sed|head|tail|less|more|rg|grep)\b`)
	writeCommandRE  = regexp.MustCompile(`(?:^|(?:&&|\|\||[;&|])\s*)(?:[A-Za-z0-9_.-]+/)?(?:cp|mv|mkdir|touch|rm|chmod|chown|install|tee)\b|\bgit\s+(?:add|mv|rm)\b|\bsed\s+-i\b|>\s*["']?[^"'\s]*[/\\]SKILL\.md\b`)
)

func inferCursorSkillName(toolName, inputJSON string) string {
	if !isCursorSkillReadTool(toolName) {
		return ""
	}
	return inferSkillNameFromJSONPaths(inputJSON)
}

func inferCodexSkillName(toolName, inputJSON string) string {
	if !isCodexShellTool(toolName) {
		return ""
	}
	cmd := skillCommandFromInput(inputJSON)
	if !looksLikeSkillReadCommand(cmd) {
		return ""
	}
	for _, path := range skillPathsFromText(cmd) {
		if name := skillNameFromPath(path); name != "" {
			return name
		}
	}
	return ""
}

func inferSkillNameFromJSONPaths(inputJSON string) string {
	trimmed := strings.TrimSpace(inputJSON)
	if trimmed == "" {
		return ""
	}
	if !gjson.Valid(trimmed) {
		for _, path := range skillPathsFromText(trimmed) {
			if name := skillNameFromPath(path); name != "" {
				return name
			}
		}
		return ""
	}

	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return ""
	}
	var found string
	var walk func(any)
	walk = func(x any) {
		if found != "" {
			return
		}
		switch t := x.(type) {
		case string:
			for _, path := range skillPathsFromText(t) {
				if name := skillNameFromPath(path); name != "" {
					found = name
					return
				}
			}
		case []any:
			for _, item := range t {
				walk(item)
				if found != "" {
					return
				}
			}
		case map[string]any:
			for _, item := range t {
				walk(item)
				if found != "" {
					return
				}
			}
		}
	}
	walk(v)
	return found
}

func isCursorSkillReadTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read", "readfile", "read_file":
		return true
	default:
		return false
	}
}

func isCodexShellTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "exec_command", "shell_command", "shell", "bash":
		return true
	default:
		return false
	}
}

func skillCommandFromInput(inputJSON string) string {
	trimmed := strings.TrimSpace(inputJSON)
	if trimmed == "" {
		return ""
	}
	if gjson.Valid(trimmed) {
		g := gjson.Parse(trimmed)
		for _, key := range []string{"cmd", "command", "script"} {
			if s := strings.TrimSpace(g.Get(key).Str); s != "" {
				return s
			}
		}
	}
	return trimmed
}

func looksLikeSkillReadCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" || !strings.Contains(cmd, "SKILL.md") {
		return false
	}
	if writeCommandRE.MatchString(cmd) {
		return false
	}
	return readCommandRE.MatchString(cmd)
}

func skillPathsFromText(text string) []string {
	matches := skillPathRE.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	paths := make([]string, 0, len(matches))
	for _, m := range matches {
		if !skillPathMatchHasBoundary(text, m[1]) {
			continue
		}
		for i := 2; i < len(m); i += 2 {
			if m[i] >= 0 && m[i+1] >= 0 {
				paths = append(paths, text[m[i]:m[i+1]])
				break
			}
		}
	}
	return paths
}

func skillPathMatchHasBoundary(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	switch text[end] {
	case ' ', '\t', '\n', '\r', '"', '\'', ';', '&', '|', ')', '}', ']':
		return true
	default:
		return false
	}
}

func skillNameFromPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !isSkillMarkdownPath(path) {
		return ""
	}
	clean := filepath.Clean(path)
	if cached, ok := skillNameByPath.Load(clean); ok {
		return cached.(string)
	}

	name := skillNameFromFrontmatter(clean)
	if name == "" {
		name = skillNameFromParentDir(clean)
	}
	skillNameByPath.Store(clean, name)
	return name
}

func isSkillMarkdownPath(path string) bool {
	normalized := strings.ReplaceAll(path, "\\", "/")
	return strings.HasSuffix(normalized, "/SKILL.md") ||
		normalized == "SKILL.md"
}

func skillNameFromFrontmatter(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" || line == "..." {
			return ""
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "name" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		return value
	}
	return ""
}

func skillNameFromParentDir(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	if dir == "." || dir == string(filepath.Separator) {
		return ""
	}
	return dir
}
