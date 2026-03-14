package agent

import "unicode"

func useChinesePrompt(values ...string) bool {
	for _, value := range values {
		for _, r := range value {
			if unicode.In(r, unicode.Han) {
				return true
			}
		}
	}
	return false
}
