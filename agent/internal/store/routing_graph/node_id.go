package routinggraph

import "strings"

const nodeSep = "::"

func NodeID(capability, tool string) string {
	if capability == "" || tool == "" {
		return ""
	}
	return capability + nodeSep + tool
}

func ParseNodeID(node string) (string, string, bool) {
	if node == "" {
		return "", "", false
	}
	parts := strings.SplitN(node, nodeSep, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
