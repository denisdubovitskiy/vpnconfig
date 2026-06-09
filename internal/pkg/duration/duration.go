package duration

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration — обёртка над time.Duration для парсинга из YAML в формате
// Go duration string ("24h", "30m", "1h30m"). yaml.v3 не парсит
// time.Duration напрямую.
type Duration struct {
	time.Duration
}

// UnmarshalYAML декодирует строку формата Go duration.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML кодирует значение в Go duration string.
func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}
