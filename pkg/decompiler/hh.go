package decompiler

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// Extract 使用系统的 hh.exe 将 CHM 文件反编译到指定目录
// 策略：为了避免 hh.exe 处理特殊字符（如 &、空格、中文）出错，
// 我们先将源文件复制为临时目录下的 source.chm，处理完后再删除。
func Extract(chmPath string, outputDir string) error {
	funcName := "Extract"

	// 1. 获取绝对路径
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

	// 2. 确保输出目录存在
	if _, err := os.Stat(absOutputDir); os.IsNotExist(err) {
		err := os.MkdirAll(absOutputDir, 0755)
		if err != nil {
			slog.Error("创建输出目录失败", "func", funcName, "dir", absOutputDir, "err", err)
			return fmt.Errorf("创建目录失败: %w", err)
		}
	}

	// 3. 检查 hh.exe
	hhPath, err := exec.LookPath("hh.exe")
	if err != nil {
		slog.Error("未找到 hh.exe", "func", funcName, "err", err)
		return fmt.Errorf("缺少 hh.exe: %w", err)
	}

	// 定义临时安全文件名
	safeChmPath := filepath.Join(absOutputDir, "legacy_temp.chm")

	slog.Info("正在创建安全副本以规避文件名问题", "func", funcName, "origin", absChmPath, "temp", safeChmPath)

	if err := copyFile(absChmPath, safeChmPath); err != nil {
		slog.Error("创建副本失败", "func", funcName, "err", err)
		return fmt.Errorf("复制文件失败: %w", err)
	}

	// 确保函数结束时清理临时文件
	defer func() {
		slog.Info("清理临时文件", "func", funcName, "file", safeChmPath)
		_ = os.Remove(safeChmPath)
	}()

	// 4. 执行命令 (针对 safeChmPath 执行)
	slog.Info("开始调用 hh.exe 反编译", "func", funcName, "target", safeChmPath)

	// 命令格式: hh.exe -decompile <输出目录> <CHM路径>
	cmd := exec.Command(hhPath, "-decompile", absOutputDir, safeChmPath)

	// 捕获输出，hh.exe 即使失败也可能返回 code 0，所以一定要看 output
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// 如果有输出内容，记录下来（hh.exe 正常情况下通常是静默的，有输出往往意味着警告或错误）
	if len(outputStr) > 0 {
		slog.Info("hh.exe 输出信息", "func", funcName, "output", outputStr)
	}

	if err != nil {
		slog.Error("反编译命令执行异常", "func", funcName, "err", err, "output", outputStr)
		return fmt.Errorf("反编译失败: %w", err)
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
