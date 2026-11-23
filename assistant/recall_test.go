package assistant

import (
	"strings"
	"testing"
)

func TestQueryWordsKeepsTheStemOfAName(t *testing.T) {
	words := strings.Join(queryWords("Скажи мне ФИО руководителя Бородина?"), " ")
	if !strings.Contains(words, "руково") || !strings.Contains(words, "бороди") {
		t.Fatalf("got %s", words)
	}
	if strings.Contains(words, "скажи") || strings.Contains(words, "мне") {
		t.Fatalf("stop words stayed: %s", words)
	}
}
