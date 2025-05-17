package database

import (
	"github.com/pkg/errors"
	"strings"
)

type OrderBy string

const (
	OrderByAsc  = OrderBy("ASC")
	OrderByDesc = OrderBy("DESC")
)

func (o *OrderBy) String() string {
	if o == nil {
		return "ASC"
	}

	return string(*o)
}

type PageResult[T any] struct {
	Results []T
	HasMore bool
	Cursor  string
	Error   error
}

// SplitOrderByParam is a helper function that can be used for query params to take the field plus
// direction as a single string value. e.g. "created_at DESC" This method will split the two parts
// and return the field and the order. If the direction is omitted, ASC will be assumed. If the param
// is empty or the param contains two parts but the order is invalid, this method will return an error
func SplitOrderByParam(p string) (field string, orderBy OrderBy, err error) {
	if len(p) == 0 {
		return "", OrderByAsc, errors.New("invalid order by field")
	}

	if strings.Contains(p, " ") {
		parts := strings.Split(p, " ")
		if len(parts) != 2 {
			return "", OrderByAsc, errors.New("invalid order by field")
		}

		field = parts[0]
		orderBy = OrderBy(strings.ToUpper(parts[1]))

		if orderBy != OrderByAsc && orderBy != OrderByDesc {
			return "", OrderByAsc, errors.New("invalid order by")
		}

		return field, orderBy, nil
	} else {
		return p, OrderByAsc, nil
	}
}
