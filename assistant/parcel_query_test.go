package assistant

import "testing"

func TestMatchLargestPlotInTalnakh(t *testing.T) {
	query, ok := matchParcelQuestion("покажи мне самый большой земельный участок в Талнахе")
	if !ok || query.District != "Талнах" || query.Kind != compareAreaDesc {
		t.Fatalf("got %+v %v", query, ok)
	}
}

func TestMatchCheapestPlotInOganer(t *testing.T) {
	query, ok := matchParcelQuestion("Какой самый дешёвый участок в Оганере?")
	if !ok || query.District != "Оганер" || query.Kind != compareCostAsc {
		t.Fatalf("got %+v %v", query, ok)
	}
}

func TestDocumentQuestionIsNotAParcelCompare(t *testing.T) {
	if _, ok := matchParcelQuestion("какой балл у участка в Талнахе"); ok {
		t.Fatal("a document question should stay with the documents")
	}
}

func TestTwoDistrictsAreNotAParcelCompare(t *testing.T) {
	if _, ok := matchParcelQuestion("самый большой участок в Талнахе и Кайеркане"); ok {
		t.Fatal("two districts should not pick one")
	}
}

func TestFormatRubGroupsThousands(t *testing.T) {
	if got := formatRub(4442361.07); got != "4 442 361,07 ₽" {
		t.Fatalf("got %s", got)
	}
	if got := formatHa(12.5); got != "12,5 га" {
		t.Fatalf("got %s", got)
	}
}
