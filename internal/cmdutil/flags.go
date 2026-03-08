package cmdutil

import "strings"

type OptionalString struct {
	set   bool
	value string
}

func (o *OptionalString) String() string {
	return o.value
}

func (o *OptionalString) Set(value string) error {
	o.set = true
	o.value = value
	return nil
}

func (o *OptionalString) IsSet() bool {
	return o.set
}

func (o *OptionalString) Value() string {
	return o.value
}

func SplitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}
