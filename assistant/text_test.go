package assistant

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestPlainTextRejectsUnknownExtension(t *testing.T) {
	_, err := textFromUpload("note.pdf", []byte("балл"))
	if err == nil {
		t.Fatal("expected an error for pdf")
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
