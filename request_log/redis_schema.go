package request_log

import (
	"fmt"
	"github.com/google/uuid"
)

const (
	fieldType                = "t"
	fieldRequestId           = "id"
	fieldCorrelationId       = "cid"
	fieldTimestamp           = "ts"
	fieldDurationMs          = "dur"
	fieldConnectionId        = "cionid"
	fieldConnectorType       = "ctort"
	fieldConnectorId         = "ctorid"
	fieldConnectorVersion    = "ctorv"
	fieldMethod              = "m"
	fieldHost                = "h"
	fieldScheme              = "sch"
	fieldPath                = "u"
	fieldResponseStatusCode  = "sc"
	fieldResponseError       = "err"
	fieldRequestHttpVersion  = "reqv"
	fieldRequestSizeBytes    = "reqsz"
	fieldRequestMimeTypes    = "reqmt"
	fieldResponseHttpVersion = "rspv"
	fieldResponseSizeBytes   = "rspsz"
	fieldResponseMimeTypes   = "rspmt"
)

func redisLogKey(requestId uuid.UUID) string {
	return fmt.Sprintf("rl:%s", requestId.String())
}

func redisLogDetailKey(requestId uuid.UUID) string {
	return fmt.Sprintf("rld:%s", requestId.String())
}
