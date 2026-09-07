package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = humanDurationType{}
	_ basetypes.StringValuableWithSemanticEquals = humanDurationValue{}
)

// humanDurationType treats syntactically different Go duration strings as
// equal when they represent the same duration. The API's HumanDuration
// serializer expands values such as "1m" to "1m0s" on read.
type humanDurationType struct {
	basetypes.StringType
}

func (humanDurationType) String() string {
	return "authproxy.HumanDurationType"
}

func (humanDurationType) ValueType(context.Context) attr.Value {
	return humanDurationValue{}
}

func (t humanDurationType) Equal(other attr.Type) bool {
	otherType, ok := other.(humanDurationType)
	return ok && t.StringType.Equal(otherType.StringType)
}

func (humanDurationType) ValueFromString(
	_ context.Context,
	value basetypes.StringValue,
) (basetypes.StringValuable, diag.Diagnostics) {
	return humanDurationValue{StringValue: value}, nil
}

func (t humanDurationType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	baseValue, err := t.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}
	stringValue, ok := baseValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected human duration value type %T", baseValue)
	}
	return humanDurationValue{StringValue: stringValue}, nil
}

type humanDurationValue struct {
	basetypes.StringValue
}

func newHumanDurationValue(value string) humanDurationValue {
	return humanDurationValue{StringValue: basetypes.NewStringValue(value)}
}

func (humanDurationValue) Type(context.Context) attr.Type {
	return humanDurationType{}
}

func (value humanDurationValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(humanDurationValue)
	return ok && value.StringValue.Equal(otherValue.StringValue)
}

func (value humanDurationValue) StringSemanticEquals(
	_ context.Context,
	other basetypes.StringValuable,
) (bool, diag.Diagnostics) {
	otherValue, ok := other.(humanDurationValue)
	if !ok {
		return false, nil
	}
	currentDuration, currentErr := time.ParseDuration(value.ValueString())
	otherDuration, otherErr := time.ParseDuration(otherValue.ValueString())
	if currentErr != nil || otherErr != nil {
		return false, nil
	}
	return currentDuration == otherDuration, nil
}
