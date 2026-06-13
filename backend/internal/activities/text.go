package activities

import "strings"

func truncateRunes(value string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func mindMapMarkdown(root MindMapNode) string {
	var builder strings.Builder
	writeMindMapMarkdownNode(&builder, root, 0)
	return strings.TrimSpace(builder.String())
}

func writeMindMapMarkdownNode(builder *strings.Builder, node MindMapNode, depth int) {
	label := strings.TrimSpace(node.Label)
	if label == "" {
		label = "Untitled"
	}
	builder.WriteString(strings.Repeat("  ", depth))
	builder.WriteString("- ")
	builder.WriteString(label)
	builder.WriteString("\n")
	for _, child := range node.Children {
		writeMindMapMarkdownNode(builder, child, depth+1)
	}
}
