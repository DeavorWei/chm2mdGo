package decompiler

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Extract(chmPath string, outputDir string) error {
	funcName := "Extract"

	absChmPath, err := filepath.Abs(chmPath)
	if err != nil {
		slog.Error("无法获取CHM绝对路径", "func", funcName, "err", err)
		return fmt.Errorf("路径错误: %w", err)
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		slog.Error("无法获取输出绝对路径", "func", funcName, "err", err)
		return fmt.Errorf("路径错误: %w", err)
	}

	if isCacheValid(chmPath, absOutputDir) {
		slog.Info("缓存命中，跳过反编译", "func", funcName)
		return nil
	}

	slog.Info("缓存无效或不存在，开始反编译", "func", funcName)

	if _, err := os.Stat(absOutputDir); err == nil {
		slog.Info("清理旧的输出目录", "func", funcName, "dir", absOutputDir)
		if err := os.RemoveAll(absOutputDir); err != nil {
			slog.Error("清理旧目录失败", "func", funcName, "err", err)
			return fmt.Errorf("清理旧目录失败: %w", err)
		}
	}

	hhPath, err := exec.LookPath("hh.exe")
	if err != nil {
		slog.Error("未找到 hh.exe", "func", funcName, "err", err)
		return fmt.Errorf("缺少 hh.exe: %w", err)
	}

	parentDir := filepath.Dir(absOutputDir)
	tempWorkDir := filepath.Join(parentDir, "temp_chm_extract_workdir")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(tempWorkDir); os.IsNotExist(err) {
			break
		}
		tempWorkDir = filepath.Join(parentDir, fmt.Sprintf("temp_chm_extract_workdir_%d", i))
	}

	if err := os.MkdirAll(tempWorkDir, 0755); err != nil {
		slog.Error("创建临时工作目录失败", "func", funcName, "dir", tempWorkDir, "err", err)
		return fmt.Errorf("创建临时工作目录失败: %w", err)
	}
	slog.Info("使用临时工作目录", "func", funcName, "temp_dir", tempWorkDir)

	safeChmPath := filepath.Join(tempWorkDir, "legacy_temp.chm")

	slog.Info("正在创建安全副本", "func", funcName, "origin", absChmPath, "temp", safeChmPath)

	if err := copyFile(absChmPath, safeChmPath); err != nil {
		slog.Error("创建副本失败", "func", funcName, "err", err)
		_ = os.RemoveAll(tempWorkDir)
		return fmt.Errorf("复制文件失败: %w", err)
	}

	defer func() {
		slog.Info("清理临时文件", "func", funcName, "file", safeChmPath)
		_ = os.Remove(safeChmPath)
	}()

	slog.Info("开始调用 hh.exe 反编译", "func", funcName, "target", safeChmPath)

	cmd := exec.Command(hhPath, "-decompile", tempWorkDir, safeChmPath)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if len(outputStr) > 0 {
		slog.Info("hh.exe 输出信息", "func", funcName, "output", outputStr)
	}

	if err != nil {
		slog.Error("反编译命令执行异常", "func", funcName, "err", err, "output", outputStr)
		_ = os.RemoveAll(tempWorkDir)
		return fmt.Errorf("反编译失败: %w", err)
	}

	var hhcFound bool
	filepath.Walk(tempWorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".hhc") {
			hhcFound = true
		}
		return nil
	})

	if !hhcFound {
		slog.Error("反编译后未找到HHC文件，可能解压失败", "func", funcName)
		_ = os.RemoveAll(tempWorkDir)
		return fmt.Errorf("反编译失败: 未生成有效文件")
	}

	slog.Info("重命名临时目录到目标目录", "func", funcName, "from", tempWorkDir, "to", absOutputDir)
	if err := os.Rename(tempWorkDir, absOutputDir); err != nil {
		slog.Error("重命名目录失败", "func", funcName, "err", err)
		_ = os.RemoveAll(tempWorkDir)
		return fmt.Errorf("重命名目录失败: %w", err)
	}

	if err := writeCacheMeta(chmPath, absOutputDir); err != nil {
		slog.Warn("写入缓存元数据失败", "func", funcName, "err", err)
	}

	slog.Info("反编译步骤完成", "func", funcName)
	return nil
}

// copyFile 简单的文件复制辅助函数
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
