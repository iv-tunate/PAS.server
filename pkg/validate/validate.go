package validate

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
)

type Validatable interface {
	Validate() error
}

func BindAndValidate(c echo.Context, dst Validatable) error {
	if err := c.Bind(dst); err != nil {
		return errors.New("request body could not be parsed")
	}
	if err := dst.Validate(); err != nil {
		return err
	}
	return nil
}

func Required(fields map[string]string) error {
	var missing []string
	for name, value := range fields {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return errors.New("missing required field(s): " + strings.Join(missing, ", "))
	}
	return nil
}

// MaxLen checks length in characters (not bytes), so names and messages
// with accents or emoji are counted the way a person would count them.
func MaxLen(name, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%s is too long (max %d characters)", name, max)
	}
	return nil
}

// Email accepts a plain address like jane@example.com and nothing fancier
// ("Jane <jane@example.com>" is rejected on purpose).
func Email(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > 254 {
		return errors.New("email is not valid")
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value || !strings.Contains(value[strings.LastIndex(value, "@"):], ".") {
		return errors.New("email is not valid")
	}
	return nil
}

// Phone is deliberately forgiving: people type +234 801 234 5678,
// 0801-234-5678 or (0801) 2345678. It checks the characters and that there
// are enough digits to be a real number.
func Phone(value string) error {
	value = strings.TrimSpace(value)
	digits := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return errors.New("phone number is not valid")
		}
	}
	if digits < 7 || digits > 15 {
		return errors.New("phone number is not valid")
	}
	return nil
}

// HTTPSURL allows an empty value (field not filled in) or an https link.
// It stops "javascript:" and similar from ever reaching a page that renders
// the value as a link or image.
func HTTPSURL(name, value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%s must be a full https:// link", name)
	}
	if len(value) > 600 {
		return fmt.Errorf("%s is too long", name)
	}
	return nil
}
