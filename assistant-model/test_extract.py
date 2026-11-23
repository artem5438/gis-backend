import io
import unittest

from pypdf import PdfReader, PdfWriter

from extract import extract_pdf


def minimal_pdf(text: str) -> bytes:
    objects = [
        "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
        "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n",
        "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]\n"
        "/Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n",
        None,
        "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
    ]
    stream = f"BT /F1 12 Tf 100 700 Td ({text}) Tj ET"
    objects[3] = f"4 0 obj\n<< /Length {len(stream)} >>\nstream\n{stream}\nendstream\nendobj\n"
    header = b"%PDF-1.4\n"
    body = b""
    offsets = [0]
    for item in objects:
        offsets.append(len(header) + len(body))
        body += item.encode("ascii")
    xref_at = len(header) + len(body)
    lines = ["xref\n0 6\n", "0000000000 65535 f \n"]
    for offset in offsets[1:]:
        lines.append(f"{offset:010d} 00000 n \n")
    trailer = f"trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{xref_at}\n%%EOF\n"
    return header + body + "".join(lines).encode("ascii") + trailer.encode("ascii")


class ExtractPdfTest(unittest.TestCase):
    def test_reads_text_layer(self):
        self.assertIn("Bally", extract_pdf(minimal_pdf("Bally")))

    def test_blank_page_has_no_text(self):
        with self.assertRaises(ValueError) as caught:
            extract_pdf(minimal_pdf(""))
        self.assertIn("нет текста", str(caught.exception))

    def test_password_is_refused(self):
        reader = PdfReader(io.BytesIO(minimal_pdf("Secret")))
        writer = PdfWriter()
        writer.append(reader)
        writer.encrypt("secret")
        buffer = io.BytesIO()
        writer.write(buffer)
        with self.assertRaises(ValueError) as caught:
            extract_pdf(buffer.getvalue())
        self.assertIn("паролем", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
