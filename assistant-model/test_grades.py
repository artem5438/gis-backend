import unittest

from server import (
    GRADE_OTHER,
    GRADE_SUITABLE,
    GRADE_THIN,
    GRADE_UNREAD,
    chunks_for_answer,
    grades_from_model,
)


CHUNKS = [
    {"number": 1, "text": "Балл не является ценой в рублях."},
    {"number": 2, "text": "Оклада в документе нет."},
]


class GradeReadTest(unittest.TestCase):
    def test_string_number_stays_suitable(self):
        grades = grades_from_model(
            {"grades": [{"number": "1", "grade": "годится"}]},
            CHUNKS,
        )
        self.assertEqual(grades[0]["grade"], GRADE_SUITABLE)
        self.assertEqual(grades[1]["grade"], GRADE_UNREAD)

    def test_unknown_word_is_not_replaced_with_rejection(self):
        grades = grades_from_model(
            {"grades": [{"number": 1, "grade": "не годится"}]},
            CHUNKS[:1],
        )
        self.assertEqual(grades[0]["grade"], GRADE_UNREAD)
        self.assertNotEqual(grades[0]["grade"], GRADE_OTHER)

    def test_explicit_rejection_stays(self):
        grades = grades_from_model(
            {
                "grades": [
                    {"number": 1, "grade": "не подходит"},
                    {"number": 2, "grade": "мало данных"},
                ]
            },
            CHUNKS,
        )
        self.assertEqual([item["grade"] for item in grades], [GRADE_OTHER, GRADE_THIN])
        self.assertEqual(chunks_for_answer(CHUNKS, grades), [])

    def test_suitable_chunk_is_used_without_unread(self):
        grades = grades_from_model({"grades": [{"number": "1", "grade": "подходит"}]}, CHUNKS)
        chosen = chunks_for_answer(CHUNKS, grades)
        self.assertEqual([item["number"] for item in chosen], [1])

    def test_unread_chunk_reaches_the_answer_when_nothing_is_suitable(self):
        grades = grades_from_model({"grades": [{"number": 1, "grade": "не годится"}]}, CHUNKS)
        chosen = chunks_for_answer(CHUNKS, grades)
        self.assertEqual([item["number"] for item in chosen], [1, 2])


if __name__ == "__main__":
    unittest.main()
