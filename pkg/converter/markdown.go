package converter

import (
	"chm2md/pkg/encoding"
	"fmt"
	"log/slog"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
)

// ConvertFile 读取指定路径的 HTML 文件，清洗后转换为 Markdown 字符串
func ConvertFile(htmlPath string) (string, error) {
	// 1. 读取并转码
	htmlContent, err := encoding.ReadFileAsUTF8(htmlPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	if len(htmlContent) == 0 {
		return "", nil
	}

	// 2. 解析 DOM
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("解析DOM失败: %w", err)
	}

	// ==========================================================
	// DOM 清洗与结构调整
	// ==========================================================

	// 1. 【核心修改】移除常规垃圾，并新增移除图片 (img)
	// 只要在这里删除了 img 标签，生成的 Markdown 里就绝对不会出现 ![](...)
	doc.Find("script, style, link, object, meta, iframe, img").Remove()
	doc.Find("[style*='display:none']").Remove()

	// 2. 表格预处理 (处理 td 里的 p 标签，防止表格破裂)
	doc.Find("td, th").Each(func(i int, s *goquery.Selection) {
		// 处理 p 标签：替换为内容 + 换行符
		s.Find("p").Each(func(j int, p *goquery.Selection) {
			html, _ := p.Html()
			p.ReplaceWithHtml(html + "\n")
		})

		// 处理 br 标签：转换为换行符
		s.Find("br").Each(func(j int, br *goquery.Selection) {
			br.ReplaceWithHtml("\n")
		})

		// 清理 HTML 格式换行（仅清理 td/th 直接子节点的文本换行）
		html, _ := s.Html()
		// 压缩多余空白，但保留有意添加的换行符
		html = strings.ReplaceAll(html, "\r\n", "\n")
		html = strings.ReplaceAll(html, "\r", "\n")
		// 移除连续多余换行（超过2个换行压缩为2个）
		for strings.Contains(html, "\n\n\n") {
			html = strings.ReplaceAll(html, "\n\n\n", "\n\n")
		}
		s.SetHtml(html)
	})

	// 3. 处理超链接 <a> -> [纯文本]
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			s.ReplaceWithSelection(s.Contents())
		} else {
			s.ReplaceWithHtml(fmt.Sprintf("[%s]", text))
		}
	})

	// ==========================================================
	// 转换阶段
	// ==========================================================

	cleanedHTML, err := doc.Html()
	if err != nil {
		return "", fmt.Errorf("生成HTML失败: %w", err)
	}

	// 初始化转换器并启用表格插件
	converter := md.NewConverter("", true, nil)
	converter.Use(plugin.Table())

	markdown, err := converter.ConvertString(cleanedHTML)
	if err != nil {
		return "", fmt.Errorf("转换MD失败: %w", err)
	}

	// ==========================================================
	// 文本后处理 (过滤垃圾行)
	// ==========================================================

	lines := strings.Split(markdown, "\n")
	var finalLines []string
	lastLineWasEmpty := false

	for _, line := range lines {
		trimLine := strings.TrimSpace(line)

		// 1. 过滤父主题
		if strings.HasPrefix(trimLine, "父主题") || strings.HasPrefix(trimLine, "父主题：") {
			continue
		}
		if strings.Contains(trimLine, "父主题：") && len(trimLine) < 50 {
			continue
		}

		// 2. 过滤版权
		if strings.Contains(trimLine, "版权所有") && strings.Contains(trimLine, "华为") {
			continue
		}

		// 3. 过滤导航
		if (strings.Contains(trimLine, "上一节") || strings.Contains(trimLine, "下一节")) &&
			(strings.Contains(trimLine, "[") || strings.Contains(trimLine, "<")) {
			continue
		}

		// 4. 过滤空链接
		if trimLine == "[]" || trimLine == "[ ]" {
			continue
		}

		// 5. 压缩空行
		if trimLine == "" {
			if lastLineWasEmpty {
				continue
			}
			lastLineWasEmpty = true
		} else {
			lastLineWasEmpty = false
		}

		finalLines = append(finalLines, line)
	}

	finalContent := strings.Join(finalLines, "\n")

	if len(finalContent) == 0 && len(markdown) > 0 {
		slog.Warn("清洗后内容为空", "path", htmlPath)
	}

	return finalContent, nil
}
