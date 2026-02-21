package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLoggingConfig(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("none", func(t *testing.T) {
			var l LoggingConfig
			assert.NoError(yaml.Unmarshal([]byte(`type: none`), &l))
			assert.Equal(LoggingConfig{InnerVal: &LoggingConfigNone{
				Type: LoggingConfigTypeNone,
			}}, l)
		})
		t.Run("text minimal", func(t *testing.T) {
			var l LoggingConfig
			assert.NoError(yaml.Unmarshal([]byte(`type: text`), &l))
			assert.Equal(LoggingConfig{InnerVal: &LoggingConfigText{
				Type: LoggingConfigTypeText,
			}}, l)
		})
		t.Run("text with options", func(t *testing.T) {
			data := `
type: text
to: stderr
level: debug
source: true
`
			var l LoggingConfig
			assert.NoError(yaml.Unmarshal([]byte(data), &l))
			assert.Equal(LoggingConfig{InnerVal: &LoggingConfigText{
				Type:   LoggingConfigTypeText,
				To:     OutputStderr,
				Level:  LevelDebug,
				Source: true,
			}}, l)
		})
		t.Run("json minimal", func(t *testing.T) {
			var l LoggingConfig
			assert.NoError(yaml.Unmarshal([]byte(`type: json`), &l))
			assert.Equal(LoggingConfig{InnerVal: &LoggingConfigJson{
				Type: LoggingConfigTypeJson,
			}}, l)
		})
		t.Run("tint minimal", func(t *testing.T) {
			var l LoggingConfig
			assert.NoError(yaml.Unmarshal([]byte(`type: tint`), &l))
			assert.Equal(LoggingConfig{InnerVal: &LoggingConfigTint{
				Type: LoggingConfigTypeTint,
			}}, l)
		})
		t.Run("unknown type returns error", func(t *testing.T) {
			var l LoggingConfig
			assert.Error(yaml.Unmarshal([]byte(`type: unknown`), &l))
		})
		t.Run("missing type returns error", func(t *testing.T) {
			var l LoggingConfig
			assert.Error(yaml.Unmarshal([]byte(`level: debug`), &l))
		})
	})
}
