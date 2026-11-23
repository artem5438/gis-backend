package assistant

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestPlainTextRejectsUnknownExtension(t *testing.T) {
	_, err := textFromUpload("note.png", []byte("балл"))
	if err == nil {
		t.Fatal("expected an error for png")
	}
}

func TestUploadLimitIs50MB(t *testing.T) {
	if maxUploadBytes != 50<<20 {
		t.Fatalf("got %d", maxUploadBytes)
	}
	if err := rejectUpload("note.pdf", maxUploadBytes+1); err == nil {
		t.Fatal("expected a size error")
	}
}

func TestCSVReadsSemicolon(t *testing.T) {
	text, err := textFromUpload("table.csv", []byte("район;балл\nТалнах;70\n"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "район балл\nТалнах 70" {
		t.Fatalf("got %q", text)
	}
}

func TestXlsxReadsSharedString(t *testing.T) {
	text, err := textFromUpload("table.xlsx", xlsxWith("Кайеркан"))
	if err != nil {
		t.Fatal(err)
	}
	if text != "Кайеркан" {
		t.Fatalf("got %q", text)
	}
}

func TestPlainTextTrimsBOM(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte("  балл участка  ")...)
	text, err := textFromUpload("note.txt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if text != "балл участка" {
		t.Fatalf("got %q", text)
	}
}

func TestDocxReadsParagraphs(t *testing.T) {
	payload := docxWith("Балл не является ценой.")
	text, err := textFromUpload("note.docx", payload)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Балл не является ценой." {
		t.Fatalf("got %q", text)
	}
}

func xlsxWith(value string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	shared, err := writer.Create("xl/sharedStrings.xml")
	if err != nil {
		panic(err)
	}
	if _, err := shared.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>` +
		value + `</t></si></sst>`)); err != nil {
		panic(err)
	}
	sheet, err := writer.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		panic(err)
	}
	if _, err := sheet.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>`)); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

func docxWith(paragraph string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		panic(err)
	}
	xml := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:body><w:p><w:r><w:t>` + paragraph + `</w:t></w:r></w:p></w:body></w:document>`
	if _, err := file.Write([]byte(xml)); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}
