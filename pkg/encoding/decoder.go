package encoding

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ReadFileAsUTF8 读取文件并将内容转换为 UTF-8 字符串
// 默认假设文件是 GBK 编码（这是 CHM 的通病），如果解析出的内容看起来像 UTF-8，
// 标准库通常也能兼容，但这里我们强制用 GBK 解码器处理 HHC 文件。
func ReadFileAsUTF8(filePath string) (string, error) {
	// 1. 读取原始字节
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// 2. 尝试 GBK 解码
	// 注意：绝大多数中文 CHM 的 HHC 文件都是 GBK
	reader := transform.NewReader(bytes.NewReader(rawBytes), simplifiedchinese.GBK.NewDecoder())

	decodedBytes, err := io.ReadAll(reader)
	if err != nil {
		// 如果 GBK 解码失败，可以尝试直接返回原始字符串（可能本身就是 UTF-8）
		// 但为了简单起见，这里先报错
		return "", err
	}

	return string(decodedBytes), nil
}
