package analyzer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestWriteMessage(t *testing.T) {
	message := json.RawMessage(`{"text":"привет"}`)
	var buf bytes.Buffer

	if err := WriteMessage(&buf, message); err != nil {
		t.Fatal(err)
	}

	want := fmt.Sprintf(
		"Content-Length: %d\r\n\r\n%s",
		len(message),
		message,
	)
	if buf.String() != want {
		t.Fatalf("got %q, want %q", buf.String(), want)
	}
}

func TestWriteMessageRejectsInvalidInput(t *testing.T) {
	for _, message := range []json.RawMessage{
		nil,
		json.RawMessage(`{broken`),
		json.RawMessage(`"` + string(bytes.Repeat([]byte("a"), maxMessageBytes)) + `"`),
	} {
		var buf bytes.Buffer
		if err := WriteMessage(&buf, message); err == nil {
			t.Fatal("expected an error")
		}
		if buf.Len() != 0 {
			t.Fatal("invalid message must not be written")
		}
	}
}

func TestReadMessageSequence(t *testing.T) {
	messages := []json.RawMessage{
		json.RawMessage(`{"text":"привет"}`),
		json.RawMessage(`{"id":2,"result":null}`),
	}

	var stream bytes.Buffer
	for _, message := range messages {
		if err := WriteMessage(&stream, message); err != nil {
			t.Fatal(err)
		}
	}

	reader := bufio.NewReader(&stream)
	for _, want := range messages {
		got, err := ReadMessage(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
}

func TestReadMessageRejectsInvalidInput(t *testing.T) {
	cases := map[string]string{
		"missing length":   "\r\n",
		"duplicate length": "Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}",
		"negative length":  "Content-Length: -1\r\n\r\n",
		"oversized body":   "Content-Length: 1048577\r\n\r\n",
		"truncated body":   "Content-Length: 5\r\n\r\n{}",
		"invalid JSON":     "Content-Length: 3\r\n\r\nbad",
		"invalid newline":  "Content-Length: 2\n\n{}",
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(input))
			if _, err := ReadMessage(reader); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
