package config

func LoadConfig(in []byte) (*Root, error) {
	return UnmarshallYamlRoot(in)
}
