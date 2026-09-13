package domain

import "strings"

func ValidateContact(email, phone string) error {
	if email == "" && phone == "" {
		return ErrInvalid
	}
	if email != "" && (strings.ContainsAny(email, " \t\r\n") || !strings.Contains(email, "@")) {
		return ErrInvalid
	}
	if phone != "" && !validPhone(phone) {
		return ErrInvalid
	}
	return nil
}

func validPhone(phone string) bool {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' {
			return -1
		}
		return r
	}, phone)
	if clean == "" {
		return false
	}
	if clean[0] == '+' {
		clean = clean[1:]
	}
	if len(clean) < 6 || len(clean) > 15 {
		return false
	}
	for _, c := range clean {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
