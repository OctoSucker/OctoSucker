package planning

import (
	"strings"
	"unicode/utf8"
)

func isDirectReplyRequest(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 120 {
		return false
	}
	if mentionsExternalWork(s) {
		return false
	}
	for _, marker := range []string{
		"你好", "您好", "hello", "hi", "hey",
		"你是谁", "介绍你自己", "介绍一下你自己", "你能做什么",
		"who are you", "introduce yourself", "what can you do",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func isListToolProvidersRequest(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 120 {
		return false
	}
	hasProvider := strings.Contains(s, "provider") || strings.Contains(s, "providers")
	hasToolProvider := strings.Contains(s, "工具 provider") || strings.Contains(s, "工具provider")
	hasListIntent := strings.Contains(s, "列出") || strings.Contains(s, "有哪些") || strings.Contains(s, "list")
	return hasListIntent && (hasProvider || hasToolProvider)
}

func listToolsForProviderRequest(input string, providerIDs []string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 160 {
		return "", false
	}
	hasListIntent := strings.Contains(s, "列出") || strings.Contains(s, "有哪些") || strings.Contains(s, "list")
	hasToolIntent := strings.Contains(s, "工具") || strings.Contains(s, "tool")
	if !hasListIntent || !hasToolIntent {
		return "", false
	}
	for _, id := range providerIDs {
		pid := strings.ToLower(strings.TrimSpace(id))
		if pid != "" && strings.Contains(s, pid) {
			return id, true
		}
	}
	if provider, ok := providerForIntent(s, providerIDs); ok {
		return provider, true
	}
	return "", false
}

func isListSkillsRequest(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 120 {
		return false
	}
	hasListIntent := strings.Contains(s, "列出") || strings.Contains(s, "有哪些") || strings.Contains(s, "list")
	hasSkillIntent := strings.Contains(s, "skill") || strings.Contains(s, "技能")
	return hasListIntent && hasSkillIntent
}

func isReloadSkillsRequest(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 120 {
		return false
	}
	hasReloadIntent := strings.Contains(s, "reload") || strings.Contains(s, "重载") || strings.Contains(s, "重新加载") || strings.Contains(s, "刷新")
	hasSkillIntent := strings.Contains(s, "skill") || strings.Contains(s, "技能")
	return hasReloadIntent && hasSkillIntent
}

func readSkillRequest(input string, skillNames []string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 160 {
		return "", false
	}
	hasReadIntent := strings.Contains(s, "read") || strings.Contains(s, "查看") || strings.Contains(s, "读取") || strings.Contains(s, "打开")
	hasSkillIntent := strings.Contains(s, "skill") || strings.Contains(s, "技能")
	if !hasReadIntent || !hasSkillIntent {
		return "", false
	}
	for _, name := range skillNames {
		n := strings.ToLower(strings.TrimSpace(name))
		if n != "" && strings.Contains(s, n) {
			return name, true
		}
		if normalizedSkillRef(n) != "" && strings.Contains(normalizedSkillRef(s), normalizedSkillRef(n)) {
			return name, true
		}
	}
	return "", false
}

func normalizeSkillName(raw string, skillNames []string) (string, bool) {
	target := normalizedSkillRef(raw)
	if target == "" {
		return "", false
	}
	for _, name := range skillNames {
		if normalizedSkillRef(name) == target {
			return name, true
		}
	}
	return "", false
}

func normalizedSkillRef(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mentionsExternalWork(s string) bool {
	for _, marker := range []string{
		"运行", "执行", "命令", "文件", "目录", "搜索", "网页", "浏览",
		"发送", "工具", "provider", "tool", "telegram", "定时", "cron", "知识图", "kg", "github",
		"run ", "exec", "file", "search", "http", "https://",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
