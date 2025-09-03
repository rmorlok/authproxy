module github.com/rmorlok/authproxy

go 1.24.1

require (
	code.cloudfoundry.org/bytefmt v0.45.0
	github.com/JGLTechnologies/gin-rate-limit v1.5.4
	github.com/Masterminds/squirrel v1.5.4
	github.com/alicebob/miniredis/v2 v2.35.0
	github.com/bsm/redislock v0.9.4
	github.com/fatih/color v1.18.0
	github.com/gin-contrib/cors v1.7.6
	github.com/gin-gonic/contrib v0.0.0-20240508051311-c1c6bf0061b0
	github.com/gin-gonic/gin v1.10.1
	github.com/go-resty/resty/v2 v2.16.2
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hibiken/asynq v0.25.1
	github.com/joho/godotenv v1.5.1
	github.com/lmittmann/tint v1.0.7
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/pkg/errors v0.9.1
	github.com/redis/go-redis/v9 v9.7.3
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.40.0
	gopkg.in/h2non/gentleman-mock.v2 v2.0.0
	gopkg.in/h2non/gentleman.v2 v2.0.5
	gopkg.in/h2non/gock.v1 v1.1.2
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/sqlite v1.5.6
	gorm.io/gorm v1.25.12
	k8s.io/utils v0.0.0-20241210054802-24370beab758
)

require (
	github.com/bytedance/sonic v1.13.3 // indirect
	github.com/bytedance/sonic/loader v0.2.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.5 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/h2non/parth v0.0.0-20190131123155-b4df798d6542 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid/v2 v2.2.11 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.24 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.0 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/arch v0.18.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/Masterminds/squirrel v1.5.4 => github.com/jack-t/squirrel v1.6.0
