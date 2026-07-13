package telegrambot

import (
	"fmt"
	"strings"
)

const callbackPrefix = "hedwig"

// MaxCallbackDataLen is Telegram's hard limit (in bytes) on inline keyboard
// button callback_data; encoded callback data exceeding this is rejected by
// the Bot API when the message/keyboard is sent.
const MaxCallbackDataLen = 64

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
