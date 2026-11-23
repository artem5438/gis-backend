package assistant

import "testing"

func TestGreetingSkipsDocuments(t *testing.T) {
	reply, ok := courtesyReply("Привет!")
	if !ok || reply != "Здравствуйте. Чем могу помочь?" {
		t.Fatalf("got %q %v", reply, ok)
	}
}

func TestThanksAndBye(t *testing.T) {
	if _, ok := courtesyReply("Спасибо большое"); !ok {
		t.Fatal("thanks")
	}
	reply, ok := courtesyReply("До свидания.")
	if !ok || reply != "До свидания." {
		t.Fatalf("got %q %v", reply, ok)
	}
}

func TestQuestionIsNotCourtesy(t *testing.T) {
	if _, ok := courtesyReply("Привет, кто руководитель?"); ok {
		t.Fatal("a question should stay a question")
	}
}
