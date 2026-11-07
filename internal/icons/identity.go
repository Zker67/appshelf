package icons

import "strings"

func packedProductCode(code string) string {
	if len(code) != 38 || code[0] != '{' || code[37] != '}' {
		return ""
	}
	code = strings.ToUpper(code[1:37])
	if code[8] != '-' || code[13] != '-' || code[18] != '-' || code[23] != '-' {
		return ""
	}
	code = strings.ReplaceAll(code, "-", "")
	for _, c := range code {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return ""
		}
	}
	b := []byte(code)
	for _, span := range [][2]int{{0, 8}, {8, 12}, {12, 16}} {
		for l, r := span[0], span[1]-1; l < r; l, r = l+1, r-1 {
			b[l], b[r] = b[r], b[l]
		}
	}
	for i := 16; i < 32; i += 2 {
		b[i], b[i+1] = b[i+1], b[i]
	}
	return string(b)
}

func registryWingetID(key string) string {
	key = strings.ToLower(key[strings.LastIndex(key, `\`)+1:])
	id, ok := strings.CutSuffix(key, "_microsoft.winget.source_8wekyb3d8bbwe")
	if !ok {
		return ""
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return ""
		}
	}
	return id
}
