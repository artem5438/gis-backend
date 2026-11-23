package assistant

import "strings"

func courtesyReply(question string) (string, bool) {
	text := strings.ToLower(strings.ReplaceAll(question, "ё", "е"))
	text = strings.Trim(text, " \t\r\n.,!?;:…\"'«»")
	switch text {
	case "привет", "здравствуй", "здравствуйте", "добрый день", "доброе утро", "добрый вечер", "хай", "hello", "hi":
		return "Здравствуйте. Чем могу помочь?", true
	case "спасибо", "спасибо большое", "благодарю":
		return "Пожалуйста. Если есть вопрос по участку или по документам, напишите его.", true
	case "пока", "до свидания", "до встречи":
		return "До свидания.", true
	default:
		return "", false
	}
}
