package helpers

import (
	"bytes"
	"encoding/json"
	"io"
)

func jsonMarshal(v interface{}) (io.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}
