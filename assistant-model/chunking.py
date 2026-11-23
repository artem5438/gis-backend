"""Cut a document into fixed token windows using the embedder tokenizer offsets."""

CHUNK_SIZE = 256
CHUNK_OVERLAP = 40

PASSAGE_PREFIX = "passage: "
QUERY_PREFIX = "query: "


def split_by_offsets(text, offsets, size=CHUNK_SIZE, overlap=CHUNK_OVERLAP):
    if size <= 0 or not 0 <= overlap < size:
        raise ValueError("нужны size > 0 и 0 <= overlap < size")
    pieces = []
    start = 0
    total = len(offsets)
    while start < total:
        end = min(start + size, total)
        char_start = offsets[start][0]
        char_end = offsets[end - 1][1]
        fragment = text[char_start:char_end].strip()
        if fragment:
            pieces.append(fragment)
        if end == total:
            break
        start = end - overlap
    return pieces


def offsets_of(tokenizer, text):
    encoded = tokenizer(
        text,
        add_special_tokens=False,
        return_offsets_mapping=True,
        truncation=False,
    )
    return encoded["offset_mapping"]
