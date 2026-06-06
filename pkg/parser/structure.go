package parser

import (
	"fmt"
	"strings"
)

// Node 代表目录树中的一个节点（章节或目录）
type Node struct {
	Title    string  // 章节标题
	Path     string  // 对应的 HTML 文件路径 (相对路径)
	Children []*Node // 子章节
}

// IsFolder 判断当前节点是否仅仅是个文件夹（没有对应的内容页）
func (n *Node) IsFolder() bool {
	return n.Path == "" || strings.TrimSpace(n.Path) == ""
}

// PrintTree 辅助函数：打印树结构用于调试
func (n *Node) PrintTree(level int) {
	prefix := strings.Repeat("  ", level)
	icon := "📄"
	if n.IsFolder() {
		icon = "📂"
	}
	fmt.Printf("%s%s %s (%s)\n", prefix, icon, n.Title, n.Path)

	for _, child := range n.Children {
		child.PrintTree(level + 1)
	}
}
