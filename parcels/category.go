package parcels

import "strings"

// CategoryTitle matches the eight legend titles used on the map.
// An unknown text stays in the last group, the same way the map paints it.
func CategoryTitle(value string) string {
	text := strings.ReplaceAll(strings.ToLower(value), "ё", "е")
	switch {
	case strings.Contains(text, "сельскохозяй"):
		return "Земли сельскохозяйственного назначения"
	case strings.Contains(text, "населен"):
		return "Земли населенных пунктов"
	case strings.Contains(text, "промышлен") || strings.Contains(text, "энергетик") || strings.Contains(text, "транспорт") || strings.Contains(text, "специальн"):
		return "Земли промышленности и иного назначения"
	case strings.Contains(text, "особо охраня"):
		return "Земли особо охраняемых территорий"
	case strings.Contains(text, "лесн"):
		return "Земли лесного фонда"
	case strings.Contains(text, "водн"):
		return "Земли водного фонда"
	case strings.Contains(text, "запас"):
		return "Земли запаса"
	default:
		return "Категория не установлена"
	}
}
