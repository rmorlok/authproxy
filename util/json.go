package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func MustPrettyJSON(v interface{}) string {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}

	return string(jsonData)
}

func MustPrettyPrintJSON(v interface{}) {
	fmt.Println(string(MustPrettyJSON(v)))
}

// JsonToReader converts a struct to a reader by serializing it to JSON.
func JsonToReader(v interface{}) io.Reader {
	jsonData, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return bytes.NewReader(jsonData)
}
