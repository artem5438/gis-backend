"""Text layer of a PDF. Scans and password-locked files are refused."""

import io


def extract_pdf(data: bytes) -> str:
    if not data:
        raise ValueError("файл пустой")
    try:
        from pypdf import PdfReader
    except ImportError as exc:
        raise ValueError("разбор pdf недоступен") from exc
    try:
        reader = PdfReader(io.BytesIO(data))
    except Exception as exc:
        raise ValueError("файл pdf не читается") from exc
    if reader.is_encrypted:
        try:
            opened = reader.decrypt("")
        except Exception as exc:
            raise ValueError("файл pdf закрыт паролем") from exc
        if int(opened) == 0:
            raise ValueError("файл pdf закрыт паролем")
    parts = []
    try:
        for page in reader.pages:
            parts.append(page.extract_text() or "")
    except Exception as exc:
        raise ValueError("файл pdf не читается") from exc
    text = "\n".join(part.strip() for part in parts if part and part.strip()).strip()
    if not text:
        raise ValueError("в pdf нет текста")
    return text
