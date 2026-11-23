import unittest

from grounding import invented_name


class GroundingTest(unittest.TestCase):
    def test_full_name_must_be_in_the_chunk(self):
        self.assertTrue(invented_name("Киселёв Иван Михайлович", "Научный руководитель: Беляев Игорь Сергеевич"))

    def test_name_from_the_chunk_stays(self):
        source = "Научный руководитель: Беляев Игорь Сергеевич, к.э.н."
        self.assertFalse(invented_name("Беляев Игорь Сергеевич.", source))

    def test_ordinary_sentence_is_not_a_name(self):
        self.assertFalse(invented_name("Документ описывает оценку участка.", "оценка участка"))

    def test_surname_after_a_pointer_must_be_in_the_chunk(self):
        self.assertTrue(invented_name("Это Киселёв.", "Научный руководитель: Беляев Игорь Сергеевич"))

    def test_name_in_another_case_stays(self):
        source = "Бородин Артём Сергеевич Геоинформационная система оценки инвестиционного потенциала Арктического города"
        answer = "У Бородина Артёма Сергеевича тема: Геоинформационная система оценки инвестиционного потенциала Арктического города."
        self.assertFalse(invented_name(answer, source))

    def test_refusal_has_no_invented_name(self):
        self.assertFalse(invented_name("Извините, у меня нет этой информации.", ""))


if __name__ == "__main__":
    unittest.main()
