package merger

import (
	"chm2md/pkg/converter"
	"chm2md/pkg/parser"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8" // 【引入 utf8 包】用于正确统计中文字符
)

// LimitCharCount 单个文件最大字符数限制 (45万字)
const LimitCharCount = 480000

// Process 根据指定的层级合并策略生成 Markdown 文件
func Process(nodes []*parser.Node, tempDir, outDir string, mergeLevel int) error {
	for _, node := range nodes {
		if err := visit(node, tempDir, outDir, 1, mergeLevel); err != nil {
			return err
		}
	}
	return nil
}

// visit 递归访问节点
func visit(node *parser.Node, srcBase, dstBase string, currentDepth, targetLevel int) error {
	// 情况 A：当前深度 < 目标合并层级
	if currentDepth < targetLevel {
		if len(node.Children) > 0 {
			newDir := filepath.Join(dstBase, sanitizeFilename(node.Title))
			if err := os.MkdirAll(newDir, 0755); err != nil {
				return err
			}
			for _, child := range node.Children {
				if err := visit(child, srcBase, newDir, currentDepth+1, targetLevel); err != nil {
					return err
				}
			}
		}
		// 孤儿叶子节点
		if node.Path != "" && len(node.Children) == 0 {
			return generateSingleFile(node, srcBase, dstBase)
		}
		return nil
	}

	// 情况 B：当前深度 == 目标合并层级 -> 合并节点
	if currentDepth == targetLevel {
		baseFileName := sanitizeFilename(node.Title) + ".md"
		dstPath := filepath.Join(dstBase, baseFileName)

		slog.Info("正在合并处理", "root_node", node.Title)

		content, err := collectContent(node, srcBase, 1)
		if err != nil {
			return err
		}

		return saveContentWithSplitting(dstPath, content)
	}

	return nil
}

// saveContentWithSplitting 检查内容长度，如果过长则分片保存
func saveContentWithSplitting(filePath string, content string) error {
	// 【核心修改 1】使用 RuneCountInString 统计真实的字符数 (包含中文)
	totalRunes := utf8.RuneCountInString(content)

	// 1. 如果未超过限制，直接保存
	if totalRunes <= LimitCharCount {
		slog.Info("生成文件 (未分片)", "file", filePath, "chars", totalRunes)
		return os.WriteFile(filePath, []byte(content), 0644)
	}

	// 2. 超过限制，开始分片
	lines := strings.Split(content, "\n")

	fileExt := filepath.Ext(filePath)
	fileNameWithoutExt := strings.TrimSuffix(filePath, fileExt)

	partCounter := 1
	currentBuffer := strings.Builder{}
	currentRuneCount := 0 // 改名变量，明确这是字符计数

	slog.Info("内容过大，触发自动分片", "total_chars", totalRunes, "limit", LimitCharCount)

	for _, line := range lines {
		// 【核心修改 2】统计当前行的字符数 (中文算1个，英文算1个) + 1个换行符
		lineRunes := utf8.RuneCountInString(line) + 1

		// 预判：如果加上这行会超标，先保存之前的 Buffer
		if currentRuneCount+lineRunes > LimitCharCount && currentRuneCount > 0 {
			partFileName := fmt.Sprintf("%s%d%s", fileNameWithoutExt, partCounter, fileExt)

			if err := os.WriteFile(partFileName, []byte(currentBuffer.String()), 0644); err != nil {
				return fmt.Errorf("写入分片失败 %s: %w", partFileName, err)
			}
			slog.Info("生成分片文件", "file", partFileName, "chars", currentRuneCount)

			partCounter++
			currentBuffer.Reset()
			currentRuneCount = 0
		}

		currentBuffer.WriteString(line)
		currentBuffer.WriteString("\n")
		currentRuneCount += lineRunes
	}

	// 3. 写入最后一部分
	if currentRuneCount > 0 {
		partFileName := fmt.Sprintf("%s%d%s", fileNameWithoutExt, partCounter, fileExt)
		if err := os.WriteFile(partFileName, []byte(currentBuffer.String()), 0644); err != nil {
			return fmt.Errorf("写入最后一个分片失败 %s: %w", partFileName, err)
		}
		slog.Info("生成分片文件 (End)", "file", partFileName, "chars", currentRuneCount)
	}

	return nil
}

// collectContent 递归收集子树的所有内容
func collectContent(node *parser.Node, srcBase string, headerLevel int) (string, error) {
	var builder strings.Builder

	// 标题
	headerPrefix := strings.Repeat("#", headerLevel)
	builder.WriteString(fmt.Sprintf("%s %s\n\n", headerPrefix, node.Title))

	// 内容
	if node.Path != "" {
		srcPath := filepath.Join(srcBase, node.Path)
		mdContent, err := converter.ConvertFile(srcPath)
		if err != nil {
			slog.Warn("合并时转换文件失败", "file", node.Path, "err", err)
			builder.WriteString(fmt.Sprintf("> *[读取内容失败: %s]*\n\n", node.Title))
		} else {
			builder.WriteString(mdContent)
			builder.WriteString("\n\n")
		}
	}

	// 递归
	for _, child := range node.Children {
		childContent, err := collectContent(child, srcBase, headerLevel+1)
		if err != nil {
			return "", err
		}
		builder.WriteString(childContent)
	}

	return builder.String(), nil
}

func generateSingleFile(node *parser.Node, srcBase, dstBase string) error {
	if node.Path == "" {
		return nil
	}
	srcPath := filepath.Join(srcBase, node.Path)
	mdContent, err := converter.ConvertFile(srcPath)
	if err != nil {
		return err
	}
	dstPath := filepath.Join(dstBase, sanitizeFilename(node.Title)+".md")
	return saveContentWithSplitting(dstPath, mdContent)
}

func sanitizeFilename(name string) string {
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "\t", "\n"}
	for _, char := range invalid {
		name = strings.ReplaceAll(name, char, "_")
	}
	return strings.TrimSpace(name)
}
