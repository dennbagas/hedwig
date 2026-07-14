package telegrambot

import (
	"fmt"
	"strings"
)

const callbackPrefix = "hedwig"

// EncodeCallback encodes callback data in the format "hedwig:<feature>:<action>:<payload>".
func EncodeCallback(feature, action, payload string) string {
	return strings.Join([]string{callbackPrefix, feature, action, payload}, ":")
}

// DecodeCallback decodes callback data in the format "hedwig:<feature>:<action>:<payload>".
func DecodeCallback(data string) (feature, action, payload string, err error) {
	parts := strings.SplitN(data, ":", 4)
	if len(parts) != 4 || parts[0] != callbackPrefix {
		return "", "", "", fmt.Errorf("invalid callback data: %q", data)
	}
	return parts[1], parts[2], parts[3], nil
}
