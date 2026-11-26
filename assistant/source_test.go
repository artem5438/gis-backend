package assistant

import "testing"

func TestSourceRefsKeepsSuitableDocumentsOnce(t *testing.T) {
	grades := []gradeResponse{
		{Number: 1, Grade: "годится"},
		{Number: 2, Grade: "годится"},
		{Number: 3, Grade: "не разобрана"},
		{Number: 4, Grade: "не подходит"},
	}
	meta := map[int]sourceResponse{
		1: {ID: 7, Title: " программа.docx ", Kind: "docx"},
		2: {ID: 7, Title: "программа.docx", Kind: "docx"},
		3: {ID: 8, Title: "темы.csv", Kind: "csv"},
		4: {ID: 9, Title: "чужое.pdf", Kind: "pdf"},
	}
	refs := sourceRefs("Ответ есть.", grades, meta)
	if len(refs) != 1 || refs[0].ID != 7 || refs[0].Title != "программа.docx" || refs[0].Kind != "docx" {
		t.Fatalf("%#v", refs)
	}
}

func TestSourceRefsUsesUnreadDocumentsWhenNothingFits(t *testing.T) {
	grades := []gradeResponse{{Number: 1, Grade: "не разобрана"}}
	meta := map[int]sourceResponse{1: {ID: 3, Title: "заметка.txt", Kind: "txt"}}
	refs := sourceRefs("Ответ есть.", grades, meta)
	if len(refs) != 1 || refs[0].ID != 3 {
		t.Fatalf("%#v", refs)
	}
	if sourceRefs(refusal, grades, meta) != nil {
		t.Fatal("refusal kept a source")
	}
}
