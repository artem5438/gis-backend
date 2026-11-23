package assistant

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

const maxUploadBytes = 8 << 20

func textFromUpload(name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("файл пустой")
	}
	if len(data) > maxUploadBytes {
		return "", fmt.Errorf("файл больше 8 МБ")
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".txt", ".md":
		return plainText(data)
	case ".docx":
		return textFromDocx(data)
	default:
		return "", fmt.Errorf("нужен файл .txt, .md или .docx")
	}
}

func plainText(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return "", fmt.Errorf("текст должен быть в UTF-8")
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("в файле нет текста")
	}
	return text, nil
}

func textFromDocx(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("файл docx не читается")
	}
	var document *zip.File
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			document = file
			break
		}
	}
	if document == nil {
		return "", fmt.Errorf("в docx нет текста")
	}
	body, err := document.Open()
	if err != nil {
		return "", fmt.Errorf("в docx нет текста")
	}
	defer func() { _ = body.Close() }()
	text, err := wordText(body)
	if err != nil {
		return "", fmt.Errorf("в docx нет текста")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("в docx нет текста")
	}
	return text, nil
}

func wordText(source io.Reader) (string, error) {
	decoder := xml.NewDecoder(source)
	var builder strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == "p" && builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			inText = element.Name.Local == "t"
		case xml.EndElement:
			if element.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				builder.Write(element)
			}
		}
	}
	return builder.String(), nil
}
