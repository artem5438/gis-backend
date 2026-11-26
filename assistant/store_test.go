package assistant

import "testing"

func TestKindOfReadsTheStoredExtension(t *testing.T) {
	if kindOf("assistant/abc.DOCX") != "docx" || kindOf("assistant/abc") != "" {
		t.Fatal(kindOf("assistant/abc.DOCX"))
	}
}

func TestVectorLiteral(t *testing.T) {
	got := vectorLiteral([]float64{1, 0.5, -0.25})
	if got != "[1,0.5,-0.25]" {
		t.Fatalf("got %s", got)
	}
}
