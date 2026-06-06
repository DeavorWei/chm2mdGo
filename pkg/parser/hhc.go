package parser

import (
	"chm2md/pkg/encoding"
	"fmt"
	"log/slog"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParseHHC 解析 .hhc 文件并返回目录树的根节点列表
func ParseHHC(hhcPath string) ([]*Node, error) {
	funcName := "ParseHHC"

	// 1. 读取并转码
	htmlContent, err := encoding.ReadFileAsUTF8(hhcPath)
	if err != nil {
		return nil, fmt.Errorf("读取HHC文件失败: %w", err)
	}

	// 2. 加载到 goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("解析HTML结构失败: %w", err)
	}

	var roots []*Node

	// 3. 开始解析
	// 查找最外层的 <ul> 下的直接 <li>
	doc.Find("body > ul > li").Each(func(i int, s *goquery.Selection) {
		node := parseListItem(s)
		if node != nil {
			roots = append(roots, node)
		}
	})

	slog.Info("HHC解析完成", "func", funcName, "root_nodes", len(roots))
	return roots, nil
}

// parseListItem 递归解析 <li> 标签
func parseListItem(s *goquery.Selection) *Node {
	node := &Node{}

	// --- 步骤 1: 提取当前节点信息 (OBJECT) ---
	// 优先查找直接子节点
	obj := s.Children().Filter("object")
	if obj.Length() == 0 {
		// 备用：放宽搜索范围 (防止 HTML 结构畸形)
		obj = s.Find("object").First()
	}

	node.Title = getParamValue(obj, "Name")

	rawPath := getParamValue(obj, "Local")
	node.Path = strings.ReplaceAll(rawPath, "\\", "/")

	// 如果标题为空，通常是无效节点，但也可能是纯粹的容器节点，我们继续尝试找子节点
	if node.Title == "" && node.Path == "" {
		// 如果既没标题也没路径，大概率是空行或无效数据，但也可能是纯结构体
		// 我们暂且继续，如果下面也没子节点，最后返回 nil 也不迟
	}

	// --- 步骤 2: 查找子目录容器 (UL) ---

	// 策略 A (标准嵌套): <LI> <OBJECT>...</OBJECT> <UL>...</UL> </LI>
	// 这是你当前的结构，也是最常见的
	ul := s.Children().Filter("ul")

	// 策略 B (兄弟结构): <LI>...</LI> <UL>...</UL>
	// 很多老式编辑器会生成这种结构，虽然你的文件不是这样，但加个保险
	if ul.Length() == 0 {
		ul = s.NextFiltered("ul")
	}

	// --- 步骤 3: 递归遍历子节点 (LI) ---

	// 关键修改：不再使用 Find("> li")，而是使用 Children().Filter("li")
	// 这种链式调用是 Goquery 最底层的逻辑，不依赖 CSS 选择器引擎，最稳健。
	ul.Children().Filter("li").Each(func(i int, childSel *goquery.Selection) {
		childNode := parseListItem(childSel)
		if childNode != nil {
			node.Children = append(node.Children, childNode)
		}
	})

	// 如果这个节点既没有内容，也没有子节点，那它就是毫无意义的，丢弃
	if node.Title == "" && len(node.Children) == 0 {
		return nil
	}

	return node
}

// getParamValue 从 object 标签中提取 param value
func getParamValue(s *goquery.Selection, name string) string {
	val := ""
	// 使用 Children 确保只读取当前 Object 下的 param，不读歪了
	s.Children().Filter("param").Each(func(i int, param *goquery.Selection) {
		if val != "" {
			return
		}

		pName, _ := param.Attr("name")
		if strings.EqualFold(pName, name) {
			val, _ = param.Attr("value")
		}
	})
	return val
}
