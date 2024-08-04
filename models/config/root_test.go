package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFullConfig(t *testing.T) {
	assert := require.New(t)

	data := `
integrations:
  - id: google-drive
    version: 1
    display_name: Google Drive
    logo:
      public_url: https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031
    description: | 
      This integration connects Google Drive to Acme app. This has the following advantages:
      
      1. Some reason
      2. Some other reason
      3. Even better reason
    auth:
      type: OAuth2
      client_id:
        value: some-client-id
      client_secret:
        env_var: GOOGLE_DRIVE_CLIENT_SECRET
      scopes:
        - id: https://www.googleapis.com/auth/drive.readonly
          reason: |
            We nee to be able to view the files
        - id: https://www.googleapis.com/auth/drive.activity.readonly
          required: false
          reason: |
            We need to be able to see what's been going on in drive
  - id: greenhouse
    version: 1
    display_name: Greenhouse
    logo:
      base64: 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CCCCCCCCCCCCCCCPTOCCCCCCCCCCCCCCCCCCCCDOzVLmLCCCCCCCCCCCCCCCCCCF0iCBsgCCCCCCCCCCCCCCCCCD8LCCCXYCCCCCCCCCCCCCCCCCDZgCCCR5CCCCCCCCCCCCCCCCCChICCDkhCCCCCCCCCCCCCCCCCCSsv8AQ0Ywgggggggggggggggggggwgcggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggv/xAAnEQEAAgEDAgYDAQEAAAAAAAABABEhMUBRQdEQYXGBkbEgocFw4f/aAAgBAwEBPxD/AAoUVrLYC8RF07dFHl/YQtmn6rtCAWsCt28GCYK+7DnFcUxQdNqBl2IvuB4me7zhn2h9U1em1QBXEZOq3wZPIhBeGOzIaOph9o7O1+nw6saDSG4c4d9rqkHUmaM8dZnwX6TMFvBrGHsHG2BIbOsKYa/2PEt7UuBAA7eO8ywD4mWqwcGh8wG987bQUPGj1/5OsC4IrS3wwlf2D/e8mDV9zrsx81mLxQfv8HOhS/iD5RNmFbpiLxgdvwd4hUJTlNmDLrkmO0Mnb3j06fASVrCfXq+sA2x9tmKSkggtfowDEzJdPzKIQi5ree0Vdu1BpQmGvoTU9/wn/8QAHhEAAwACAgMBAAAAAAAAAAAAAAERMUAQISBBcFH/2gAIAQIBAT8Q+FxrweBazFDYlNVhfoiGG9bB4FnVa7GvDCaso1CsSux712ztlHaE9RoJXlqjUE7p5GHgmoyMNgSjcQwN0Smowm0UVsWuiF18J//EACkQAQACAQMDBAICAwEAAAAAAAEAESExQVFhcYFQkaGxIMEQMJDR4fH/2gAIAQEAAT8Q/wAIPl5TJWfLBJ8KyDzZhB3kQ9GnV2x0jVSmZQ0j6eGABybotfKsIkUiAccsL5y8+n3cUPeJD8fwr2y1ASpSMzYqPfXz6VoweYvibVBvgCUdDfUnyw9xe19MQADoj0W6Qiry7Aoh0TCVV5ZfLWamWiLN7Dd6t+lPzdExr9jMpuZC25W8oDEolF6w6x0iUHYCEXwHz09KumXSYDV/XdINugD7esUMo97gJy/Z0jt2SLfvcNZRmi692WB3GHXk4dpS8QxQZhaejHRKI2T0mrLitZMQe9waS1m6WulZ1dozKo2q2v8AInrLUqHriB81gdH9MUol0sDUGXtjwekAqVzAzpeOqF+WOofcnaUoPj8bUgo5TJDeE3Sl9Ka/fpGq6xRLEnshsORhbIE+78S0ud7Cv+C/uId6ek5vqnw4fVTXEbK2TQGj9/isweQ/8G3vBlIQJW+o/r0k8rLRgdvJ81zKJrcqMjytk4SZVTrwHA3fEbGkphJSdghIilDQcBsdWGdUNgCBmBargINlXdAOfJ+K9JQEi0NIyr8QVVW515JY5lKAjzGqrq1L9oHO9A2eYAKCELWiX2Ys8HW36vj0tyo2ikeSHkWBe03OpnvBmnZeJhJWIm2VOV4DVl/Ew2x9U0dD3ja25fTWdALcp456zQKAkLTWOSGIsAby4HAbHp2mXZi+IfFRtaGg8PmEAaEuRGfIlR5V16E4/YRkbUhSelsXWgWrwEfRuUe42dNe0PDG+R3dWVRLgxBKYoKhgfASB5dGsLqGvc9punHpAViCWp0CEpq7ch+zlgbDGQANVYbXWBc86R4ckY9i4bGOyT6jobQ+nYImIgIljKAqQ8cy2eTeZGn0cVrUDRo9+xAVBIHaOrsHLBEJqiDlmvbT8ACIjYjEL9Mt3H7MkISRawO8JGzWEN2E2Nzw6+/o1u5SLurwDBgNNsBRLBN4IkO0Zxpq54PztDsou52uve4immCSVruzq940h9xkRpPRRGmheqz8EOZmZKbj1fgt8Tr+bLFj1G47Nl/AY5BMZCEORXwL59FDeTPaDSE9ymc4P6SIrROxR8VNIRvFPH/Xopj5XkQMRgNV+bf0mcU+HoPqO0cVy18j/XotLBA3rTT9kHMxFiVb6vlA3+atgCbq0TeMrqhl97jpCurVW9W/Tx6K7FD8UfbXxHSELEbEgA2FJyRpAjWxbXw/1+d+mopjbDtr7SsxKQZm1rQ8uJa0d9Vfo2ktCznTHxp7TIh470fYpHjlmtZdHnp+ODHoV+6/Up0IX33mGYfG0db7fu+PR7laEaPI8jGaCB3K5OSWpxN0wrDH4ref9ZQeRP46YH5FT7QjNsWGN+zXzDDPQKA4CLWsxMuBun2cEf6RS1Oq+kMnzCa9Hk6TAkqHqd93fHWAEMsSx/iyWSiGJmxAe8LZtwGjxde7L1PHtXK+lvlc3tvhxDBE1VT7PiIPTwHyEPkvR+gxw20aS8uPiWu12DU7Gk3v/B5//9k=
    description: |
      This integration pushes candidates to greenhouse
    auth:
      type: api-key
`
	expected := &Root{
		Integrations: []Integration{
			{
				Id:          "google-drive",
				Version:     1,
				DisplayName: "Google Drive",
				Description: "This integration connects Google Drive to Acme app. This has the following advantages:\n\n1. Some reason\n2. Some other reason\n3. Even better reason\n",
				Logo: &ImagePublicUrl{
					PublicUrl: "https://upload.wikimedia.org/wikipedia/commons/thumb/1/12/Google_Drive_icon_%282020%29.svg/1024px-Google_Drive_icon_%282020%29.svg.png?20221103153031",
				},
				Auth: &AuthOAuth2{
					Type: AuthTypeOAuth2,
					ClientId: &SecretValue{
						Value: "some-client-id",
					},
					ClientSecret: &SecretEnvVar{
						EnvVar: "GOOGLE_DRIVE_CLIENT_SECRET",
					},
					Scopes: []Scope{
						{
							Id:       "https://www.googleapis.com/auth/drive.readonly",
							Required: true,
							Reason:   "We nee to be able to view the files\n",
						},
						{
							Id:       "https://www.googleapis.com/auth/drive.activity.readonly",
							Required: false,
							Reason:   "We need to be able to see what's been going on in drive\n",
						},
					},
				},
			},
			{
				Id:          "greenhouse",
				Version:     1,
				DisplayName: "Greenhouse",
				Description: "This integration pushes candidates to greenhouse\n",
				Logo: &ImageBase64{
					Base64: "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CCCCCCCCCCCCCCCPTOCCCCCCCCCCCCCCCCCCCCDOzVLmLCCCCCCCCCCCCCCCCCCF0iCBsgCCCCCCCCCCCCCCCCCD8LCCCXYCCCCCCCCCCCCCCCCCDZgCCCR5CCCCCCCCCCCCCCCCCChICCDkhCCCCCCCCCCCCCCCCCCSsv8AQ0Ywgggggggggggggggggggwgcggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggv/xAAnEQEAAgEDAgYDAQEAAAAAAAABABEhMUBRQdEQYXGBkbEgocFw4f/aAAgBAwEBPxD/AAoUVrLYC8RF07dFHl/YQtmn6rtCAWsCt28GCYK+7DnFcUxQdNqBl2IvuB4me7zhn2h9U1em1QBXEZOq3wZPIhBeGOzIaOph9o7O1+nw6saDSG4c4d9rqkHUmaM8dZnwX6TMFvBrGHsHG2BIbOsKYa/2PEt7UuBAA7eO8ywD4mWqwcGh8wG987bQUPGj1/5OsC4IrS3wwlf2D/e8mDV9zrsx81mLxQfv8HOhS/iD5RNmFbpiLxgdvwd4hUJTlNmDLrkmO0Mnb3j06fASVrCfXq+sA2x9tmKSkggtfowDEzJdPzKIQi5ree0Vdu1BpQmGvoTU9/wn/8QAHhEAAwACAgMBAAAAAAAAAAAAAAERMUAQISBBcFH/2gAIAQIBAT8Q+FxrweBazFDYlNVhfoiGG9bB4FnVa7GvDCaso1CsSux712ztlHaE9RoJXlqjUE7p5GHgmoyMNgSjcQwN0Smowm0UVsWuiF18J//EACkQAQACAQMDBAICAwEAAAAAAAEAESExQVFhcYFQkaGxIMEQMJDR4fH/2gAIAQEAAT8Q/wAIPl5TJWfLBJ8KyDzZhB3kQ9GnV2x0jVSmZQ0j6eGABybotfKsIkUiAccsL5y8+n3cUPeJD8fwr2y1ASpSMzYqPfXz6VoweYvibVBvgCUdDfUnyw9xe19MQADoj0W6Qiry7Aoh0TCVV5ZfLWamWiLN7Dd6t+lPzdExr9jMpuZC25W8oDEolF6w6x0iUHYCEXwHz09KumXSYDV/XdINugD7esUMo97gJy/Z0jt2SLfvcNZRmi692WB3GHXk4dpS8QxQZhaejHRKI2T0mrLitZMQe9waS1m6WulZ1dozKo2q2v8AInrLUqHriB81gdH9MUol0sDUGXtjwekAqVzAzpeOqF+WOofcnaUoPj8bUgo5TJDeE3Sl9Ka/fpGq6xRLEnshsORhbIE+78S0ud7Cv+C/uId6ek5vqnw4fVTXEbK2TQGj9/isweQ/8G3vBlIQJW+o/r0k8rLRgdvJ81zKJrcqMjytk4SZVTrwHA3fEbGkphJSdghIilDQcBsdWGdUNgCBmBargINlXdAOfJ+K9JQEi0NIyr8QVVW515JY5lKAjzGqrq1L9oHO9A2eYAKCELWiX2Ys8HW36vj0tyo2ikeSHkWBe03OpnvBmnZeJhJWIm2VOV4DVl/Ew2x9U0dD3ja25fTWdALcp456zQKAkLTWOSGIsAby4HAbHp2mXZi+IfFRtaGg8PmEAaEuRGfIlR5V16E4/YRkbUhSelsXWgWrwEfRuUe42dNe0PDG+R3dWVRLgxBKYoKhgfASB5dGsLqGvc9punHpAViCWp0CEpq7ch+zlgbDGQANVYbXWBc86R4ckY9i4bGOyT6jobQ+nYImIgIljKAqQ8cy2eTeZGn0cVrUDRo9+xAVBIHaOrsHLBEJqiDlmvbT8ACIjYjEL9Mt3H7MkISRawO8JGzWEN2E2Nzw6+/o1u5SLurwDBgNNsBRLBN4IkO0Zxpq54PztDsou52uve4immCSVruzq940h9xkRpPRRGmheqz8EOZmZKbj1fgt8Tr+bLFj1G47Nl/AY5BMZCEORXwL59FDeTPaDSE9ymc4P6SIrROxR8VNIRvFPH/Xopj5XkQMRgNV+bf0mcU+HoPqO0cVy18j/XotLBA3rTT9kHMxFiVb6vlA3+atgCbq0TeMrqhl97jpCurVW9W/Tx6K7FD8UfbXxHSELEbEgA2FJyRpAjWxbXw/1+d+mopjbDtr7SsxKQZm1rQ8uJa0d9Vfo2ktCznTHxp7TIh470fYpHjlmtZdHnp+ODHoV+6/Up0IX33mGYfG0db7fu+PR7laEaPI8jGaCB3K5OSWpxN0wrDH4ref9ZQeRP46YH5FT7QjNsWGN+zXzDDPQKA4CLWsxMuBun2cEf6RS1Oq+kMnzCa9Hk6TAkqHqd93fHWAEMsSx/iyWSiGJmxAe8LZtwGjxde7L1PHtXK+lvlc3tvhxDBE1VT7PiIPTwHyEPkvR+gxw20aS8uPiWu12DU7Gk3v/B5//9k=",
				},
				Auth: &AuthApiKey{
					Type: AuthTypeAPIKey,
				},
			},
		},
	}

	root, err := LoadConfig([]byte(data))
	assert.NoError(err)
	assert.Equal(expected, root)
}
