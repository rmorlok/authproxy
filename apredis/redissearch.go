package apredis

import "strings"

// The characters that should be escaped for RedisSearch queries.
//
// From the [RedisSearch documentation](https://redis.io/docs/latest/develop/ai/search-and-query/advanced-concepts/query_syntax/#tag-filters):
// > The following characters in tags should be escaped with a backslash (\): $, {, }, \, and |.
//
// From [redisearch-go](https://github.com/RediSearch/redisearch-go/blob/master/redisearch/document.go#L11):
// > field_tokenization = ",.<>{}[]\"':;!@#$%^&*()-+=~"
var redisearch_tokens_escape_always = []string{
	",", ".", "<", ">", "{", "}", "[", "]",
	":", ";", "!", "@", "#", "$", "^",
	"(", ")", "-", "=", "~", "&", "\"",
}

var redisearch_tokens_wildcards = []string{
	"%", "*",
}

// EscapeRedisSearchString escapes a string to be used as a value in a RedisSearch query.
func EscapeRedisSearchString(value string) string {
	value = EscapeRedisSearchStringAllowWildcards(value)
	for _, val := range redisearch_tokens_wildcards {
		value = strings.Replace(value, val, "\\"+val, -1)
	}
	return value
}

// EscapeRedisSearchStringAllowWildcards escapes a string to be used as a value in a RedisSearch query but
// allows wildcards.
func EscapeRedisSearchStringAllowWildcards(value string) string {
	for _, val := range redisearch_tokens_escape_always {
		value = strings.Replace(value, val, "\\"+val, -1)
	}
	return value
}
