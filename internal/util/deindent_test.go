package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeindent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name     string
		AllEqual []string
	}{
		{
			"empty", []string{
				"",
				"       ",
				`
`,
				`


       



`,
			},
		},
		{
			Name: "yaml",
			AllEqual: []string{
				`
					type: OAuth2
					client_id:
						value: some-client-id
					client_secret:
						env_var: GOOGLE_DRIVE_CLIENT_SECRET
					scopes:
						- id: https://www.googleapis.com/auth/drive.readonly
						  required: true
						  reason: |
							We need to be able to view the files
						- id: https://www.googleapis.com/auth/drive.activity.readonly
						  required: false
						  reason: |
							We need to be able to see what's been going on in drive
					authorization:
						endpoint: https://example.com/authorization
					token:
						endpoint: https://example.com/token`,
				`
					type: OAuth2
					client_id:
						value: some-client-id
					client_secret:
						env_var: GOOGLE_DRIVE_CLIENT_SECRET
					scopes:
						- id: https://www.googleapis.com/auth/drive.readonly
						  required: true
						  reason: |
							We need to be able to view the files
						- id: https://www.googleapis.com/auth/drive.activity.readonly
						  required: false
						  reason: |
							We need to be able to see what's been going on in drive
					authorization:
						endpoint: https://example.com/authorization
					token:
						endpoint: https://example.com/token
                `,
				`type: OAuth2
client_id:
	value: some-client-id
client_secret:
	env_var: GOOGLE_DRIVE_CLIENT_SECRET
scopes:
	- id: https://www.googleapis.com/auth/drive.readonly
	  required: true
	  reason: |
		We need to be able to view the files
	- id: https://www.googleapis.com/auth/drive.activity.readonly
	  required: false
	  reason: |
		We need to be able to see what's been going on in drive
authorization:
	endpoint: https://example.com/authorization
token:
	endpoint: https://example.com/token
`,
				`
type: OAuth2
client_id:
	value: some-client-id
client_secret:
	env_var: GOOGLE_DRIVE_CLIENT_SECRET
scopes:
	- id: https://www.googleapis.com/auth/drive.readonly
	  required: true
	  reason: |
		We need to be able to view the files
	- id: https://www.googleapis.com/auth/drive.activity.readonly
	  required: false
	  reason: |
		We need to be able to see what's been going on in drive
authorization:
	endpoint: https://example.com/authorization
token:
	endpoint: https://example.com/token
`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			for i := 1; i < len(test.AllEqual); i++ {
				require.Equal(t, Deindent(test.AllEqual[i-1]), Deindent(test.AllEqual[i]))
			}
		})
	}
}
