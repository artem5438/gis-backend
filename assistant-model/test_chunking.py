import unittest

from chunking import split_by_offsets


class SplitTest(unittest.TestCase):
    def test_overlap_keeps_the_boundary(self):
        text = "abcdefghij"
        offsets = [(index, index + 1) for index in range(len(text))]
        pieces = split_by_offsets(text, offsets, size=6, overlap=2)
        self.assertEqual(pieces, ["abcdef", "efghij"])

    def test_short_text_is_one_piece(self):
        text = "балл"
        offsets = [(0, len(text))]
        self.assertEqual(split_by_offsets(text, offsets, size=256, overlap=40), ["балл"])

    def test_empty_offsets(self):
        self.assertEqual(split_by_offsets("", [], size=256, overlap=40), [])


if __name__ == "__main__":
    unittest.main()
