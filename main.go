package main

import (
	"chm2md/pkg/decompiler"
	"chm2md/pkg/merger"
	"chm2md/pkg/parser"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"chm2md/pkg/converter"
)

func main() {
	funcName := "main"

	var chmFile string
	var outDir string = "./output"
	var level int = 0

	args := os.Args[1:]
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "-l" && i+1 < len(args) {
			if val, err := strconv.Atoi(args[i+1]); err == nil {
				level = val
				i += 2
				continue
			}
		} else if arg == "-o" && i+1 < len(args) {
			outDir = args[i+1]
			i += 2
			continue
		} else if !strings.HasPrefix(arg, "-") {
			chmFile = arg
		}
		i++
	}

	if chmFile == "" {
		slog.Error("请指定输入文件", "func", funcName)
		os.Exit(1)
	}

	tempDir := filepath.Join(outDir, "temp_source")
	finalDir := filepath.Join(outDir, "markdown")

	// 1. 反编译
	if err := decompiler.Extract(chmFile, tempDir); err != nil {
		slog.Error("反编译失败", "func", funcName, "err", err)
		os.Exit(1)
	}

	// 2. 寻找 HHC
	var hhcPath string
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".hhc") {
			hhcPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		slog.Error("搜索 HHC 文件失败", "func", funcName, "err", err)
		os.Exit(1)
	}

	if hhcPath == "" {
		slog.Error("未找到 .hhc 文件", "func", funcName)
		os.Exit(1)
	}

	// 3. 解析结构
	roots, err := parser.ParseHHC(hhcPath)
	if err != nil {
		slog.Error("解析 HHC 失败", "func", funcName, "err", err)
		os.Exit(1)
	}

	// 确保输出目录干净
	if _, err := os.Stat(finalDir); err == nil {
		slog.Info("清理旧的输出目录", "func", funcName, "dir", finalDir)
		if err := os.RemoveAll(finalDir); err != nil {
			slog.Error("清理输出目录失败", "func", funcName, "err", err)
			os.Exit(1)
		}
	}

	if err := os.MkdirAll(finalDir, 0755); err != nil {
		slog.Error("创建输出目录失败", "err", err)
		os.Exit(1)
	}

	// 4. 根据模式执行转换
	if level > 0 {
		// --- 合并模式 ---
		slog.Info("进入合并模式", "func", funcName, "level", level)
		if err := merger.Process(roots, tempDir, finalDir, level); err != nil {
			slog.Error("合并生成失败", "func", funcName, "err", err)
			os.Exit(1)
		}
	} else {
		// --- 默认模式 ---
		slog.Info("进入默认模式 (生成所有文件)", "func", funcName)
		summaryContent := "# Summary\n\n"
		for _, node := range roots {
			summaryContent += processNodeDefault(node, tempDir, finalDir, 0)
		}
		if err := os.WriteFile(filepath.Join(finalDir, "SUMMARY.md"), []byte(summaryContent), 0644); err != nil {
			slog.Error("写入 SUMMARY.md 失败", "func", funcName, "err", err)
			os.Exit(1)
		}
	}

	slog.Info("全部完成！", "func", funcName, "output", finalDir)
}

func processNodeDefault(node *parser.Node, srcBase, dstBase string, level int) string {
	indent := strings.Repeat("  ", level)

	if node.Path == "" {
		entry := fmt.Sprintf("%s* %s\n", indent, node.Title)
		for _, child := range node.Children {
			entry += processNodeDefault(child, srcBase, dstBase, level+1)
		}
		return entry
	}

	srcPath := filepath.Join(srcBase, node.Path)

	originalRelPath := node.Path
	targetRelPath := strings.TrimSuffix(originalRelPath, filepath.Ext(originalRelPath)) + ".md"
	dstPath := filepath.Join(dstBase, targetRelPath)

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		slog.Error("创建目录失败", "err", err, "path", dstPath)
		return ""
	}

	mdContent, err := converter.ConvertFile(srcPath)
	if err != nil {
		slog.Warn("转换文件失败", "path", srcPath, "err", err)
		mdContent = fmt.Sprintf("> *[转换失败: %s]*\n\n", node.Title)
	}

	if err := os.WriteFile(dstPath, []byte(mdContent), 0644); err != nil {
		slog.Error("写入文件失败", "path", dstPath, "err", err)
	}

	webPath := strings.ReplaceAll(targetRelPath, "\\", "/")
	webPath = strings.ReplaceAll(webPath, " ", "%20")
	entry := fmt.Sprintf("%s* [%s](%s)\n", indent, node.Title, webPath)

	for _, child := range node.Children {
		entry += processNodeDefault(child, srcBase, dstBase, level+1)
	}

	return entry
}
