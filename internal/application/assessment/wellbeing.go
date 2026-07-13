package assessment

import "strings"

var crisisKeywords = map[string][]string{
	"en": {
		"suicide", "suicidal", "kill myself", "end my life",
		"self harm", "self-harm", "want to die", "no reason to live",
	},
	"id": {
		"bunuh diri", "mengakhiri hidup", "menyakiti diri sendiri",
		"ingin mati", "tidak ada alasan untuk hidup", "depresi berat",
	},
}

func scanForCrisisLanguage(texts []string, locale string) bool {
	keywords := crisisKeywords["en"]
	if locale != "en" {
		if extra, ok := crisisKeywords[locale]; ok {
			keywords = append(append([]string{}, keywords...), extra...)
		}
	}

	for _, text := range texts {
		lower := strings.ToLower(text)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
	}
	return false
}

var staticFallbackText = map[string]string{
	"en": "We're unable to generate your personalized AI summary right now, but your MBTI type and GRIT score below are still fully valid — please check back later for the full analysis.",
	"id": "Ringkasan AI yang dipersonalisasi belum bisa kami tampilkan saat ini, namun tipe MBTI dan skor GRIT Anda di bawah tetap valid — silakan cek kembali nanti untuk analisis lengkapnya.",
}

func fallbackText(locale string) string {
	if text, ok := staticFallbackText[locale]; ok {
		return text
	}
	return staticFallbackText["en"]
}
