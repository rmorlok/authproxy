package util

func CoerceBool(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}

func CoerceString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
