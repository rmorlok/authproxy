package pagination

import (
	"strings"

	"github.com/pkg/errors"
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

	if !o.IsAsc() && !o.IsDesc() {
		// Protect against invalid values
		return "ASC"
	}

	return string(*o)
}

func (o *OrderBy) IsDesc() bool {
	if o == nil {
		return false
	}

	return *o == OrderByDesc
}

func (o *OrderBy) IsAsc() bool {
	if o == nil {
		// Assume asc unless specified.
		return true
	}

	return *o == OrderByAsc
}

// SplitOrderByParam is a helper function that can be used for query params to take the field plus
// direction as a single string value. e.g. "created_at DESC" This method will split the two parts
// and return the field and the order. If the direction is omitted, ASC will be assumed. If the param
// is empty or the param contains two parts but the order is invalid, this method will return an error
func SplitOrderByParam[T ~string](p string) (field T, orderBy OrderBy, err error) {
	if len(p) == 0 {
		return "", OrderByAsc, errors.New("invalid order by field")
	}

	if strings.Contains(p, " ") {
		parts := strings.Split(p, " ")
		if len(parts) != 2 {
			return "", OrderByAsc, errors.New("invalid order by field")
		}

		field = T(parts[0])
		orderBy = OrderBy(strings.ToUpper(parts[1]))

		if orderBy != OrderByAsc && orderBy != OrderByDesc {
			return "", OrderByAsc, errors.New("invalid order by")
		}

		return field, orderBy, nil
	} else {
		return T(p), OrderByAsc, nil
	}
}
