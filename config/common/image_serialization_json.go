package common

import (
	"encoding/json"
	"errors"
)

func (d *Image) MarshalJSON() ([]byte, error) {
	if d.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(d.InnerVal)
}

func (d *Image) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		d.InnerVal = nil
		return nil
	}

	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		// Extract the string without quotes and process it
		content := string(data[1 : len(data)-1])
		d.InnerVal = &ImagePublicUrl{PublicUrl: content, IsDirectString: true}

		return nil
	}

	var tmp map[string]interface{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	if _, ok := tmp["public_url"]; ok {
		d.InnerVal = &ImagePublicUrl{}
		return json.Unmarshal(data, d.InnerVal)
	}

	if _, ok := tmp["base64"]; ok {
		d.InnerVal = &ImageBase64{}
		return json.Unmarshal(data, d.InnerVal)
	}

	return errors.New("invalid structure for image type; does not match base64 or public_url")
}
