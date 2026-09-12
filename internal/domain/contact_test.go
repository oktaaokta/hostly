package domain

import "testing"

func TestValidateContact(t *testing.T) {
	cases := []struct {
		email, phone string
		wantErr      bool
	}{
		{"rita@x.com", "+62812345678", false},
		{"rita@x.com", "", false},
		{"", "+62812345678", false},
		{"", "", true},
		{"not-an-email", "", true},
		{"a b@x.com", "", true},
		{"", "notaphone", true},
		{"", "12", true},
		{"", "+62 812-3456 7890", false},
		{"", "+6281234567890123456", true},
	}
	for _, c := range cases {
		err := ValidateContact(c.email, c.phone)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateContact(%q, %q) err = %v, wantErr %v", c.email, c.phone, err, c.wantErr)
		}
	}
}
