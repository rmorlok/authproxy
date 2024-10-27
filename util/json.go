package util

import (
	"encoding/json"
	"fmt"
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
