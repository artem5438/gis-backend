package assistant

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxUploadBytes = 50 << 20

func rejectUpload(name string, size int) error {
	if size == 0 {
		return fmt.Errorf("файл пустой")
	}
	if size > maxUploadBytes {
		return fmt.Errorf("файл больше 50 МБ")
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".txt", ".md", ".docx", ".pdf", ".xlsx", ".csv":
		return nil
	default:
		return fmt.Errorf("нужен файл .txt, .md, .docx, .pdf, .xlsx или .csv")
	}
}

func textFromUpload(name string, data []byte) (string, error) {
	if err := rejectUpload(name, len(data)); err != nil {
		return "", err
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".txt", ".md":
		return plainText(data)
	case ".docx":
		return textFromDocx(data)
	case ".xlsx":
		return textFromXlsx(data)
	case ".csv":
		return textFromCSV(data)
	default:
		return "", fmt.Errorf("нужен файл .txt, .md, .docx, .pdf, .xlsx или .csv")
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

func textFromCSV(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(data) {
		return "", fmt.Errorf("текст должен быть в UTF-8")
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	reader.Comma = csvComma(data)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("файл csv не читается")
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		fields := make([]string, 0, len(record))
		for _, field := range record {
			field = strings.TrimSpace(field)
			if field != "" {
				fields = append(fields, field)
			}
		}
		if len(fields) > 0 {
			lines = append(lines, strings.Join(fields, " "))
		}
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return "", fmt.Errorf("в файле нет текста")
	}
	return text, nil
}

func csvComma(data []byte) rune {
	line := data
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		line = data[:index]
	}
	if bytes.Count(line, []byte{';'}) > bytes.Count(line, []byte{','}) {
		return ';'
	}
	return ','
}

func textFromXlsx(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("файл xlsx не читается")
	}
	files := map[string]*zip.File{}
	for _, file := range reader.File {
		files[file.Name] = file
	}
	shared, err := sharedStrings(files["xl/sharedStrings.xml"])
	if err != nil {
		return "", fmt.Errorf("файл xlsx не читается")
	}
	sheets := make([]string, 0)
	for name := range files {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			sheets = append(sheets, name)
		}
	}
	sort.Strings(sheets)
	blocks := make([]string, 0, len(sheets))
	for _, name := range sheets {
		block, err := sheetText(files[name], shared)
		if err != nil {
			return "", fmt.Errorf("файл xlsx не читается")
		}
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	text := strings.TrimSpace(strings.Join(blocks, "\n"))
	if text == "" {
		return "", fmt.Errorf("в файле нет текста")
	}
	return text, nil
}

func sharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	body, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	decoder := xml.NewDecoder(body)
	var items []string
	var current strings.Builder
	inItem := false
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "si":
				inItem = true
				current.Reset()
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "t":
				inText = false
			case "si":
				if inItem {
					items = append(items, strings.TrimSpace(current.String()))
				}
				inItem = false
			}
		case xml.CharData:
			if inItem && inText {
				current.Write(element)
			}
		}
	}
	return items, nil
}

func sheetText(file *zip.File, shared []string) (string, error) {
	body, err := file.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	decoder := xml.NewDecoder(body)
	var lines []string
	var row []string
	var cellType string
	var value strings.Builder
	inRow := false
	inValue := false
	inInline := false
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
			switch element.Name.Local {
			case "row":
				inRow = true
				row = nil
			case "c":
				cellType = ""
				for _, attr := range element.Attr {
					if attr.Name.Local == "t" {
						cellType = attr.Value
					}
				}
			case "v":
				inValue = true
				value.Reset()
			case "t":
				if cellType == "inlineStr" {
					inInline = true
					value.Reset()
				}
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "v":
				inValue = false
				if inRow {
					appendCell(&row, cellText(cellType, value.String(), shared))
				}
			case "t":
				if inInline {
					inInline = false
					if inRow {
						appendCell(&row, value.String())
					}
				}
			case "row":
				if len(row) > 0 {
					lines = append(lines, strings.Join(row, " "))
				}
				inRow = false
			}
		case xml.CharData:
			if inValue || inInline {
				value.Write(element)
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func appendCell(row *[]string, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*row = append(*row, value)
	}
}

func cellText(kind string, raw string, shared []string) string {
	raw = strings.TrimSpace(raw)
	if kind != "s" {
		return raw
	}
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || index >= len(shared) {
		return ""
	}
	return shared[index]
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
