package parcels

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"strings"
)

func renderDocx(doc reportDocument, choices reportChoices) ([]byte, error) {
	var pictures []chartPicture
	var body strings.Builder
	body.WriteString(docxParagraph(doc.Title, 32, true))
	body.WriteString(docxParagraph("Кадастровый номер: "+doc.Number, 24, true))
	body.WriteString(docxParagraph("Дата: "+doc.FormedOn, 22, false))
	author := doc.Author
	if author == "" {
		author = "не указан"
	}
	body.WriteString(docxParagraph("Сформировал: "+author, 22, false))
	body.WriteString(docxParagraph("Паспорт", 28, true))
	for _, line := range doc.Passport {
		body.WriteString(docxFact(line))
	}
	if choices.Score && doc.Score != nil {
		body.WriteString(docxParagraph(doc.Score.Title, 28, true))
		if doc.Score.Note != "" {
			body.WriteString(docxParagraph(doc.Score.Note, 20, false))
		}
		for _, line := range doc.Score.Lines {
			body.WriteString(docxFact(line))
		}
	}
	if choices.Quarter && doc.Quarter != nil {
		body.WriteString(docxParagraph(doc.Quarter.Title, 28, true))
		if doc.Quarter.Note != "" {
			body.WriteString(docxParagraph(doc.Quarter.Note, 20, false))
		}
		for _, line := range doc.Quarter.Lines {
			body.WriteString(docxFact(line))
		}
		if choices.Charts {
			for _, chart := range doc.Charts {
				body.WriteString(docxParagraph(chart.Title, 24, true))
				if chart.Kind == "pie" || chart.Kind == "donut" {
					body.WriteString(docxCircleChart(&pictures, chart))
					continue
				}
				body.WriteString(docxBarTable(&pictures, chart))
			}
		}
	}
	return zipDocx(body.String(), pictures)
}

func docxFact(line reportLine) string {
	text := docxParagraph(line.Label+": "+line.Value, 22, false)
	if line.Hint != "" {
		text += docxParagraph(line.Hint, 18, false)
	}
	return text
}

func docxParagraph(text string, halfPoints int, bold bool) string {
	weight := ""
	if bold {
		weight = "<w:b/>"
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="80" w:after="80"/></w:pPr><w:r><w:rPr>%s<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, weight, halfPoints, halfPoints, html.EscapeString(text))
}

type chartPicture struct {
	PNG    []byte
	Width  int
	Height int
}

func docxBarTable(pictures *[]chartPicture, chart reportChart) string {
	maxValue := chartMax(chart)
	var body strings.Builder
	body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/></w:tblPr>`)
	for _, slice := range chart.Slices {
		strip := barStrip(slice.Value, maxValue, slice.Color)
		if len(strip) == 0 {
			continue
		}
		*pictures = append(*pictures, chartPicture{PNG: strip, Width: 400, Height: 16})
		body.WriteString(`<w:tr>`)
		body.WriteString(docxCell(slice.Name, 4200))
		body.WriteString(`<w:tc><w:tcPr><w:tcW w:w="3200" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr><w:p>`)
		body.WriteString(docxDrawing(len(*pictures), 58, 3.2))
		body.WriteString(`</w:p></w:tc>`)
		body.WriteString(docxCell(ruFloat(slice.Value, 2), 1400))
		body.WriteString(`</w:tr>`)
	}
	body.WriteString(`</w:tbl>`)
	return body.String()
}

func docxCircleChart(pictures *[]chartPicture, chart reportChart) string {
	png, width, height, err := circlePNG(chart, chart.Kind == "donut")
	if err != nil || len(png) == 0 {
		return ""
	}
	*pictures = append(*pictures, chartPicture{PNG: png, Width: width, Height: height})
	var legend strings.Builder
	for _, slice := range chart.Slices {
		legend.WriteString(docxParagraph(slice.Name+" — "+ruFloat(slice.Value, 2), 20, false))
	}
	return `<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/></w:tblPr><w:tr>` +
		`<w:tc><w:tcPr><w:tcW w:w="2600" w:type="dxa"/></w:tcPr><w:p>` + docxDrawing(len(*pictures), 42, 42) + `</w:p></w:tc>` +
		`<w:tc><w:tcPr><w:tcW w:w="6400" w:type="dxa"/></w:tcPr>` + legend.String() + `</w:tc>` +
		`</w:tr></w:tbl>`
}

func docxCell(text string, width int) string {
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr><w:spacing w:before="40" w:after="40"/></w:pPr><w:r><w:rPr><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, width, html.EscapeString(text))
}

func docxDrawing(index int, widthMM, heightMM float64) string {
	emuX := int(widthMM * 36000)
	emuY := int(heightMM * 36000)
	return fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="chart%d"/><wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="chart%d.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rId%d"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`, emuX, emuY, index, index, index, index, index+1, emuX, emuY)
}

func zipDocx(body string, pictures []chartPicture) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{
		"[Content_Types].xml":          docxContentTypes(len(pictures)),
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            docxDocument(body),
		"word/_rels/document.xml.rels": docxRels(len(pictures)),
	}
	for name, content := range files {
		if err := writeZip(archive, name, []byte(content)); err != nil {
			return nil, err
		}
	}
	for index, picture := range pictures {
		if err := writeZip(archive, fmt.Sprintf("word/media/chart%d.png", index+1), picture.PNG); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeZip(archive *zip.Writer, name string, content []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(content)
	return err
}

func docxDocument(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">` +
		`<w:body>` + body + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="1134"/></w:sectPr></w:body></w:document>`
}

func docxContentTypes(pictures int) string {
	extra := ""
	if pictures > 0 {
		extra = `<Default Extension="png" ContentType="image/png"/>`
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` + extra + `<Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`
}

func docxRels(pictures int) string {
	var links strings.Builder
	links.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for index := 1; index <= pictures; index++ {
		fmt.Fprintf(&links, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/chart%d.png"/>`, index+1, index)
	}
	links.WriteString(`</Relationships>`)
	return links.String()
}
