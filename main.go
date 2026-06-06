package main

import (
	"chm2md/pkg/decompiler"
	"chm2md/pkg/merger"
	"chm2md/pkg/parser"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"chm2md/pkg/converter"
)

func main() {
	funcName := "main"

	chmFile := flag.String("i", "", "输入的 CHM 文件路径")
	outDir := flag.String("o", "./output", "输出目录路径")
	level := flag.Int("l", 0, "合并层级：0=不合并(默认)，1=按根目录合并，2=按二级目录合并...")
	flattenDir := flag.String("d", "", "扁平化指定目录：将所有子目录文件提取到该目录下")
	flag.Parse()

	if *flattenDir != "" {
		if err := flattenDirectory(*flattenDir); err != nil {
			slog.Error("扁平化失败", "func", funcName, "err", err)
			os.Exit(1)
		}
		slog.Info("扁平化完成", "func", funcName, "dir", *flattenDir)
		return
	}

	if *chmFile == "" {
		slog.Error("请指定输入文件，使用 -i 参数，或使用 -d 扁平化目录", "func", funcName)
		flag.Usage()
		os.Exit(1)
	}

	// 从 CHM 文件名提取基础名（不含扩展名）
	chmBaseName := filepath.Base(*chmFile)
	chmBaseName = strings.TrimSuffix(chmBaseName, filepath.Ext(chmBaseName))

	tempDir := filepath.Join(*outDir, "temp_"+chmBaseName)
	finalDir := filepath.Join(*outDir, "md_"+chmBaseName)

	// 1. 反编译
	if err := decompiler.Extract(*chmFile, tempDir); err != nil {
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
	if *level > 0 {
		// --- 合并模式 ---
		slog.Info("进入合并模式", "func", funcName, "level", *level)
		if err := merger.Process(roots, tempDir, finalDir, *level); err != nil {
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

func flattenDirectory(targetDir string) error {
	funcName := "flattenDirectory"

	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("获取目录绝对路径失败: %w", err)
	}

	if _, err := os.Stat(absTargetDir); os.IsNotExist(err) {
		return fmt.Errorf("目录不存在: %s", absTargetDir)
	}

	for {
		movedCount, err := moveFilesToRoot(absTargetDir)
		if err != nil {
			return err
		}

		if movedCount == 0 {
			break
		}

		slog.Info("移动文件完成", "func", funcName, "count", movedCount)
	}

	if err := removeEmptyDirs(absTargetDir); err != nil {
		return err
	}

	if hasRemainingSubdirs(absTargetDir) {
		return fmt.Errorf("扁平化后仍存在非空子目录，可能存在文件名冲突")
	}

	return nil
}

func moveFilesToRoot(rootDir string) (int, error) {
	funcName := "moveFilesToRoot"
	movedCount := 0

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Dir(path) == rootDir {
			return nil
		}

		targetPath := filepath.Join(rootDir, filepath.Base(path))

		if _, err := os.Stat(targetPath); err == nil {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			ext := filepath.Ext(path)
			counter := 1
			for {
				newName := fmt.Sprintf("%s_%d%s", base, counter, ext)
				targetPath = filepath.Join(rootDir, newName)
				if _, err := os.Stat(targetPath); os.IsNotExist(err) {
					break
				}
				counter++
			}
			slog.Warn("文件名冲突，自动重命名", "func", funcName, "original", filepath.Base(path), "new", filepath.Base(targetPath))
		}

		if err := os.Rename(path, targetPath); err != nil {
			return fmt.Errorf("移动文件失败 %s -> %s: %w", path, targetPath, err)
		}

		slog.Debug("移动文件", "func", funcName, "from", path, "to", targetPath)
		movedCount++
		return nil
	})

	return movedCount, err
}

func removeEmptyDirs(rootDir string) error {
	funcName := "removeEmptyDirs"

	for {
		removedCount := 0
		err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if !info.IsDir() || path == rootDir {
				return nil
			}

			isEmpty, err := isDirEmpty(path)
			if err != nil {
				return nil
			}

			if isEmpty {
				if err := os.Remove(path); err != nil {
					return nil
				}
				slog.Debug("删除空目录", "func", funcName, "dir", path)
				removedCount++
			}

			return nil
		})

		if err != nil {
			return err
		}

		if removedCount == 0 {
			break
		}
	}

	return nil
}

func isDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err != nil {
		return true, nil
	}

	return false, nil
}

func hasRemainingSubdirs(rootDir string) bool {
	hasSubdir := false
	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && path != rootDir {
			isEmpty, _ := isDirEmpty(path)
			if !isEmpty {
				hasSubdir = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return hasSubdir
}
