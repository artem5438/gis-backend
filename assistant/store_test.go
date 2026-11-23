package assistant

import "testing"

func TestVectorLiteral(t *testing.T) {
	got := vectorLiteral([]float64{1, 0.5, -0.25})
	if got != "[1,0.5,-0.25]" {
		t.Fatalf("got %s", got)
	}
}
