package decompiler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CacheFileName = ".chm2md.cache"

type CacheMeta struct {
	SourcePath string `json:"source_path"`
	SourceMtime int64 `json:"source_mtime"`
	SourceSize int64  `json:"source_size"`
	CreatedAt string  `json:"created_at"`
}

func isCacheValid(chmPath, outputDir string) bool {
	funcName := "isCacheValid"

	cacheFilePath := filepath.Join(outputDir, CacheFileName)

	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		slog.Debug("缓存文件不存在或读取失败", "func", funcName, "err", err)
		return false
	}

	var meta CacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		slog.Debug("缓存元数据解析失败", "func", funcName, "err", err)
		return false
	}

	absChmPath, _ := filepath.Abs(chmPath)
	if meta.SourcePath != absChmPath {
		slog.Debug("源文件路径不匹配", "func", funcName, "cached", meta.SourcePath, "current", absChmPath)
		return false
	}

	info, err := os.Stat(chmPath)
	if err != nil {
		slog.Debug("无法获取源文件信息", "func", funcName, "err", err)
		return false
	}

	if info.ModTime().Unix() != meta.SourceMtime {
		slog.Debug("源文件修改时间变化", "func", funcName, "cached", meta.SourceMtime, "current", info.ModTime().Unix())
		return false
	}

	if info.Size() != meta.SourceSize {
		slog.Debug("源文件大小变化", "func", funcName, "cached", meta.SourceSize, "current", info.Size())
		return false
	}

	hhcPath := ""
	filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(strings.ToLower(path)) == ".hhc" {
			hhcPath = path
		}
		return nil
	})

	if hhcPath == "" {
		slog.Debug("缓存目录中未找到HHC文件", "func", funcName)
		return false
	}

	return true
}

func writeCacheMeta(chmPath, outputDir string) error {
	funcName := "writeCacheMeta"

	absChmPath, err := filepath.Abs(chmPath)
	if err != nil {
		return fmt.Errorf("获取绝对路径失败: %w", err)
	}

	info, err := os.Stat(chmPath)
	if err != nil {
		return fmt.Errorf("获取文件信息失败: %w", err)
	}

	meta := CacheMeta{
		SourcePath:  absChmPath,
		SourceMtime: info.ModTime().Unix(),
		SourceSize:  info.Size(),
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化元数据失败: %w", err)
	}

	cacheFilePath := filepath.Join(outputDir, CacheFileName)
	if err := os.WriteFile(cacheFilePath, data, 0644); err != nil {
		return fmt.Errorf("写入缓存文件失败: %w", err)
	}

	slog.Info("缓存元数据已写入", "func", funcName, "file", cacheFilePath)
	return nil
}

func ClearCache(outputDir string) error {
	cacheFilePath := filepath.Join(outputDir, CacheFileName)
	return os.Remove(cacheFilePath)
}
