package analyzer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const maxMessageBytes = 1 << 20 // 1 МиБ

func WriteMessage(w io.Writer, message json.RawMessage) error {
	if len(message) == 0 || len(message) > maxMessageBytes {
		return fmt.Errorf("invalid LSP message size: %d", len(message))
	}
	if !json.Valid(message) {
		return fmt.Errorf("invalid LSP JSON")
	}

	if _, err := fmt.Fprintf(
		w,
		"Content-Length: %d\r\n\r\n",
		len(message),
	); err != nil {
		return fmt.Errorf("write LSP header: %w", err)
	}

	n, err := w.Write(message)
	if err != nil {
		return fmt.Errorf("write LSP body: %w", err)
	}
	if n != len(message) {
		return io.ErrShortWrite
	}
	return nil
}

func ReadMessage(r *bufio.Reader) (json.RawMessage, error) {
	const maxHeaderBytes = 8192

	length := -1
	headerBytes := 0

	for {
		line, err := r.ReadSlice('\n')
		if err != nil {
			return nil, fmt.Errorf("read LSP header: %w", err)
		}

		headerBytes += len(line)
		if headerBytes > maxHeaderBytes {
			return nil, fmt.Errorf("LSP header too large")
		}
		if !bytes.HasSuffix(line, []byte("\r\n")) {
			return nil, fmt.Errorf("invalid LSP header ending")
		}

		text := string(line[:len(line)-2])
		if text == "" {
			break
		}

		name, value, ok := strings.Cut(text, ":")
		if !ok {
			return nil, fmt.Errorf("invalid LSP header")
		}
		if strings.EqualFold(name, "Content-Length") {
			if length != -1 {
				return nil, fmt.Errorf("duplicate Content-Length")
			}

			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil || length <= 0 || length > maxMessageBytes {
				return nil, fmt.Errorf("invalid Content-Length")
			}
		}
	}

	if length == -1 {
		return nil, fmt.Errorf("missing Content-Length")
	}

	message := make(json.RawMessage, length)
	if _, err := io.ReadFull(r, message); err != nil {
		return nil, fmt.Errorf("read LSP body: %w", err)
	}
	if !json.Valid(message) {
		return nil, fmt.Errorf("invalid LSP JSON")
	}

	return message, nil
}
