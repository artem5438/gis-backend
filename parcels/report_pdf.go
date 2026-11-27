package parcels

import (
	"bytes"
	_ "embed"

	"github.com/go-pdf/fpdf"
)

//go:embed font/DejaVuSans.ttf
var reportFont []byte

func renderPDF(doc reportDocument, choices reportChoices) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddUTF8FontFromBytes("dejavu", "", reportFont)
	pdf.AddPage()
	pdf.SetTextColor(20, 20, 20)
	writeLine(pdf, doc.Title, 16, 8, false)
	writeLine(pdf, "Кадастровый номер: "+doc.Number, 12, 6, false)
	writeLine(pdf, "Дата: "+doc.FormedOn, 11, 5, false)
	author := doc.Author
	if author == "" {
		author = "не указан"
	}
	writeLine(pdf, "Сформировал: "+author, 11, 6, false)
	writeHeading(pdf, "Паспорт")
	for _, line := range doc.Passport {
		writeFact(pdf, line)
	}
	if choices.Score && doc.Score != nil {
		writeHeading(pdf, doc.Score.Title)
		writeNote(pdf, doc.Score.Note)
		for _, line := range doc.Score.Lines {
			writeFact(pdf, line)
		}
	}
	if choices.Quarter && doc.Quarter != nil {
		writeHeading(pdf, doc.Quarter.Title)
		writeNote(pdf, doc.Quarter.Note)
		for _, line := range doc.Quarter.Lines {
			writeFact(pdf, line)
		}
		if choices.Charts {
			for _, chart := range doc.Charts {
				writeChart(pdf, chart)
			}
		}
	}
	if pdf.Err() {
		return nil, pdf.Error()
	}
	var buffer bytes.Buffer
	if err := pdf.Output(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeHeading(pdf *fpdf.Fpdf, text string) {
	pdf.Ln(3)
	writeLine(pdf, text, 14, 7, false)
}

func writeNote(pdf *fpdf.Fpdf, text string) {
	if text == "" {
		return
	}
	pdf.SetTextColor(92, 92, 92)
	writeLine(pdf, text, 10, 5, false)
	pdf.SetTextColor(20, 20, 20)
}

func writeFact(pdf *fpdf.Fpdf, line reportLine) {
	pdf.Ln(1.5)
	pdf.SetTextColor(92, 92, 92)
	writeLine(pdf, line.Label, 10, 5, false)
	pdf.SetTextColor(20, 20, 20)
	writeLine(pdf, line.Value, 12, 6, false)
	if line.Hint == "" {
		return
	}
	pdf.SetTextColor(92, 92, 92)
	writeLine(pdf, line.Hint, 9, 4, false)
	pdf.SetTextColor(20, 20, 20)
}

func writeLine(pdf *fpdf.Fpdf, text string, size, height float64, inset bool) {
	pdf.SetFont("dejavu", "", size)
	x := 16.0
	width := 178.0
	if inset {
		x = 20
		width = 174
	}
	pdf.SetX(x)
	pdf.MultiCell(width, height, text, "", "L", false)
}

func writeChart(pdf *fpdf.Fpdf, chart reportChart) {
	if len(chart.Slices) == 0 {
		return
	}
	pdf.Ln(2)
	writeLine(pdf, chart.Title, 12, 6, false)
	if chart.Kind == "pie" || chart.Kind == "donut" {
		writeCircleChart(pdf, chart)
		return
	}
	writeBarRows(pdf, chart)
}

func writeBarRows(pdf *fpdf.Fpdf, chart reportChart) {
	maxValue := chartMax(chart)
	for _, slice := range chart.Slices {
		if pdf.GetY() > 270 {
			pdf.AddPage()
		}
		top := pdf.GetY()
		pdf.SetXY(16, top)
		pdf.SetFont("dejavu", "", 10)
		pdf.SetTextColor(20, 20, 20)
		pdf.MultiCell(68, 4.6, slice.Name, "", "L", false)
		bottom := pdf.GetY()
		shade := parseHex(slice.Color)
		pdf.SetFillColor(236, 236, 236)
		pdf.Rect(88, top+1, 68, 3.4, "F")
		pdf.SetFillColor(int(shade.R), int(shade.G), int(shade.B))
		pdf.Rect(88, top+1, barShare(slice.Value, maxValue, 68), 3.4, "F")
		pdf.SetXY(160, top)
		pdf.CellFormat(34, 5, ruFloat(slice.Value, 2), "", 0, "L", false, 0, "")
		if bottom < top+6 {
			bottom = top + 6
		}
		pdf.SetY(bottom + 1.2)
	}
}

func writeCircleChart(pdf *fpdf.Fpdf, chart reportChart) {
	png, _, _, err := circlePNG(chart, chart.Kind == "donut")
	if err != nil || len(png) == 0 {
		return
	}
	name := "chart-" + chart.Title
	options := fpdf.ImageOptions{ImageType: "PNG"}
	if pdf.RegisterImageOptionsReader(name, options, bytes.NewReader(png)) == nil || pdf.Err() {
		return
	}
	top := pdf.GetY()
	const side = 46.0
	if top+side > 280 {
		pdf.AddPage()
		top = pdf.GetY()
	}
	pdf.ImageOptions(name, 16, top, side, side, false, options, 0, "")
	cursor := top
	pdf.SetFont("dejavu", "", 10)
	pdf.SetTextColor(20, 20, 20)
	for _, slice := range chart.Slices {
		pdf.SetXY(68, cursor)
		pdf.MultiCell(124, 4.8, slice.Name+" — "+ruFloat(slice.Value, 2), "", "L", false)
		cursor = pdf.GetY()
	}
	if cursor < top+side {
		cursor = top + side
	}
	pdf.SetY(cursor + 3)
}

func barShare(value, max, room float64) float64 {
	if max <= 0 || value <= 0 {
		return 0
	}
	share := room * value / max
	if share < 1.2 {
		return 1.2
	}
	if share > room {
		return room
	}
	return share
}
