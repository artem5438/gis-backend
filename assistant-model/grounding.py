"""Drop an answer that names a person the suitable chunks do not contain."""

import re

NAME = re.compile(r"[А-ЯЁ][а-яё-]{2,}")
TOKEN = re.compile(r"[А-ЯЁа-яё-]{2,}|[.!?]")
STOP = {
    "это", "эта", "эти", "вот", "для", "при", "как", "что", "кто", "где",
    "там", "тут", "он", "она", "они", "его", "ее", "ему", "если", "тогда",
    "здесь", "документ", "статья", "участок", "ответ", "вопрос",
    "извините", "здравствуйте", "пожалуйста", "до", "свидания",
}


def _fold(text: str) -> str:
    return text.lower().replace("ё", "е")


def _same_stem(left: str, right: str) -> bool:
    shared = 0
    for a, b in zip(left, right):
        if a != b:
            break
        shared += 1
    return shared >= 4


def _known(word: str, folded: str) -> bool:
    name = _fold(word)
    if name in folded:
        return True
    for source_word in re.findall(r"[а-я-]{4,}", folded):
        if _same_stem(name, source_word):
            return True
    return False


def invented_name(answer: str, sources: str) -> bool:
    folded = _fold(sources)
    groups = []
    current = []
    held = None

    def close(group):
        if group:
            groups.append(group)

    for token in TOKEN.findall(answer):
        if token in ".!?":
            if held and _fold(held) not in STOP:
                close([held])
            close(current)
            current = []
            held = None
            continue
        if not NAME.fullmatch(token):
            if held and _fold(held) not in STOP:
                close([held])
            close(current)
            current = []
            held = None
            continue
        if held is None and not current:
            held = token
            continue
        if held is not None:
            if _fold(held) in STOP:
                current = [token]
            else:
                current = [held, token]
            held = None
            continue
        current.append(token)
    if held is not None and _fold(held) not in STOP:
        close([held])
    close(current)

    words = [token for token in TOKEN.findall(answer) if token not in ".!?"]
    if words and all(NAME.fullmatch(token) for token in words) and words not in groups:
        groups.append(words)

    for group in groups:
        if any(not _known(word, folded) for word in group):
            return True
    return False
