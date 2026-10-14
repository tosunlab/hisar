package cliprotocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrMessageTooLarge = errors.New("cliprotocol: message larger than 16 MiB")

func ReadMessage(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}
	message := make([]byte, length)
	if _, err := io.ReadFull(reader, message); err != nil {
		return nil, err
	}
	return message, nil
}

func WriteMessage(writer io.Writer, message []byte) error {
	if len(message) > MaxMessageSize {
		return ErrMessageTooLarge
	}
	frame := make([]byte, 4+len(message))
	binary.BigEndian.PutUint32(frame, uint32(len(message)))
	copy(frame[4:], message)
	_, err := writer.Write(frame)
	return err
}

func WriteJSON(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return WriteMessage(writer, encoded)
}

func ReadJSON(reader io.Reader, value any) error {
	message, err := ReadMessage(reader)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(message, value); err != nil {
		return fmt.Errorf("cliprotocol: %w", err)
	}
	return nil
}
