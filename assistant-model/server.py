"""Local assistant models: embeddings, chunk grades, and the answer.

The Go server stores files and vectors. This process never opens PostgreSQL.
"""

import json
import os
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

from extract import extract_pdf
from chunking import (
    CHUNK_OVERLAP,
    CHUNK_SIZE,
    PASSAGE_PREFIX,
    QUERY_PREFIX,
    offsets_of,
    split_by_offsets,
)

HOST = os.environ.get("ASSISTANT_HOST", "0.0.0.0")
PORT = int(os.environ.get("ASSISTANT_PORT", "8090"))
EMBED_MODEL = os.environ.get("EMBED_MODEL", "intfloat/multilingual-e5-small")
REFUSAL = "В документах этого нет."
MAX_BODY = 80 * 1024 * 1024

GRADE_SUITABLE = "годится"
GRADE_THIN = "мало данных"
GRADE_OTHER = "не подходит"
GRADE_ALIASES = {
    "годится": GRADE_SUITABLE,
    "подходит": GRADE_SUITABLE,
    "мало данных": GRADE_THIN,
    "сомнительно": GRADE_THIN,
    "не подходит": GRADE_OTHER,
}

GRADE_PROMPT = """
Оцени каждый фрагмент относительно вопроса.
Верни только JSON без пояснений:
{"grades":[{"number":1,"grade":"годится"}]}
Поле grade — ровно одно из трёх значений:
- годится — во фрагменте есть данные, по которым можно ответить на вопрос
- мало данных — тема та же, но нужных данных нет
- не подходит — фрагмент про другое
Число оценок равно числу фрагментов. Номера те же, что у фрагментов.
""".strip()

ANSWER_PROMPT = """
Ответь на вопрос только по приведённым фрагментам.
Не добавляй факты, цены, номера и сроки, которых во фрагментах нет.
Пиши по-русски, коротко.
""".strip()

model_lock = threading.Lock()
embedder = None
writer = None


def gguf_path():
    configured = os.environ.get("QWEN_GGUF", "").strip()
    if configured:
        return configured
    return str(
        Path.home()
        / ".lmstudio/models/lmstudio-community/Qwen2.5-7B-Instruct-Q4_K_M/Qwen2.5-7B-Instruct-Q4_K_M.gguf"
    )


def load_models():
    global embedder, writer
    from llama_cpp import Llama
    from sentence_transformers import SentenceTransformer

    print(f"loading embedder {EMBED_MODEL}", flush=True)
    embedder = SentenceTransformer(EMBED_MODEL, device="cpu")
    path = gguf_path()
    if not Path(path).is_file():
        raise SystemExit(f"файл модели ответа не найден: {path}")
    print(f"loading writer {path}", flush=True)
    common = {"model_path": path, "n_ctx": 4096, "verbose": False}
    try:
        writer = Llama(n_gpu_layers=-1, **common)
    except Exception as exc:
        print(f"gpu layers unavailable ({exc}); loading on cpu", flush=True)
        writer = Llama(n_gpu_layers=0, **common)
    print("models ready", flush=True)


def complete(system, user, max_tokens):
    with model_lock:
        result = writer.create_chat_completion(
            messages=[
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            temperature=0,
            max_tokens=max_tokens,
        )
    message = result["choices"][0]["message"]
    return (message.get("content") or "").strip()


def embed_passages(pieces):
    vectors = embedder.encode(
        [PASSAGE_PREFIX + piece for piece in pieces],
        batch_size=8,
        normalize_embeddings=True,
        show_progress_bar=False,
        convert_to_numpy=True,
    )
    return [vector.tolist() for vector in vectors]


def prepare(text):
    cleaned = (text or "").strip()
    if not cleaned:
        return []
    pieces = split_by_offsets(
        cleaned,
        offsets_of(embedder.tokenizer, cleaned),
        CHUNK_SIZE,
        CHUNK_OVERLAP,
    )
    if not pieces:
        return []
    vectors = embed_passages(pieces)
    return [
        {"number": number, "text": piece, "vector": vector}
        for number, (piece, vector) in enumerate(zip(pieces, vectors), start=1)
    ]


def embed_query(text):
    cleaned = (text or "").strip()
    if not cleaned:
        raise ValueError("пустой вопрос")
    vector = embedder.encode(
        [QUERY_PREFIX + cleaned],
        normalize_embeddings=True,
        show_progress_bar=False,
        convert_to_numpy=True,
    )[0]
    return vector.tolist()


def parse_object(raw):
    start = raw.find("{")
    end = raw.rfind("}")
    if start < 0 or end <= start:
        raise ValueError("в ответе модели нет JSON")
    return json.loads(raw[start : end + 1])


def grade_chunks(question, chunks):
    lines = [f"Вопрос: {question}", "Фрагменты:"]
    for chunk in chunks:
        lines.append(f"{chunk['number']}. {chunk['text']}")
    raw = complete(GRADE_PROMPT, "\n".join(lines), max_tokens=300)
    parsed = parse_object(raw)
    by_number = {}
    for item in parsed.get("grades", []):
        number = item.get("number")
        grade = GRADE_ALIASES.get(str(item.get("grade", "")).strip().lower())
        if isinstance(number, int) and grade:
            by_number[number] = grade
    grades = []
    for chunk in chunks:
        grades.append(
            {
                "number": chunk["number"],
                "grade": by_number.get(chunk["number"], GRADE_OTHER),
            }
        )
    return grades


def answer(question, chunks):
    if not chunks:
        return {"grades": [], "answer": REFUSAL}
    grades = grade_chunks(question, chunks)
    suitable_numbers = {item["number"] for item in grades if item["grade"] == GRADE_SUITABLE}
    suitable = [chunk for chunk in chunks if chunk["number"] in suitable_numbers]
    if not suitable:
        return {"grades": grades, "answer": REFUSAL}
    lines = [f"Вопрос: {question}", "Фрагменты:"]
    for chunk in suitable:
        lines.append(f"{chunk['number']}. {chunk['text']}")
    text = complete(ANSWER_PROMPT, "\n".join(lines), max_tokens=500)
    if not text:
        text = REFUSAL
    return {"grades": grades, "answer": text}


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        print(f"{self.address_string()} {fmt % args}", flush=True)

    def do_GET(self):
        if self.path.split("?", 1)[0] != "/health":
            self.respond(404, {"error": "не найдено"})
            return
        self.respond(200, {"status": "ok"})

    def do_POST(self):
        path = self.path.split("?", 1)[0]
        if path == "/extract-pdf":
            try:
                self.respond(200, {"text": extract_pdf(self.read_bytes())})
            except ValueError as exc:
                self.respond(400, {"error": str(exc)})
            except Exception as exc:
                print(f"request failed: {exc}", flush=True)
                self.respond(500, {"error": "файл pdf не читается"})
            return
        try:
            payload = self.read_json()
        except ValueError as exc:
            self.respond(400, {"error": str(exc)})
            return
        try:
            if path == "/prepare":
                self.respond(200, {"chunks": prepare(payload.get("text", ""))})
            elif path == "/embed-query":
                self.respond(200, {"vector": embed_query(payload.get("text", ""))})
            elif path == "/answer":
                chunks = payload.get("chunks") or []
                normalized = []
                for chunk in chunks:
                    number = chunk.get("number")
                    text = chunk.get("text") or ""
                    if not isinstance(number, int) or not str(text).strip():
                        raise ValueError("у фрагмента нужны номер и текст")
                    normalized.append({"number": number, "text": str(text).strip()})
                self.respond(200, answer(str(payload.get("question") or "").strip(), normalized))
            else:
                self.respond(404, {"error": "не найдено"})
        except ValueError as exc:
            self.respond(400, {"error": str(exc)})
        except Exception as exc:
            print(f"request failed: {exc}", flush=True)
            self.respond(500, {"error": "модель не ответила"})

    def read_bytes(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length < 0 or length > MAX_BODY:
            raise ValueError("слишком большой запрос")
        return self.rfile.read(length)

    def read_json(self):
        raw = self.read_bytes()
        if not raw:
            return {}
        try:
            body = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise ValueError("некорректный JSON") from exc
        if not isinstance(body, dict):
            raise ValueError("ожидался объект JSON")
        return body

    def respond(self, status, payload):
        raw = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


def main():
    load_models()
    server = HTTPServer((HOST, PORT), Handler)
    print(f"listening on {HOST}:{PORT}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
