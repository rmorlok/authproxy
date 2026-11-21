package runtime

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// textCodecTestHost isolates the native conversion algorithms from constructor
// and input admission; each test gets a fresh VM and shared execution ledger.
func textCodecTestHost(outputLimit int) *utilityHost {
	ctx, err := WithExecutionBudget(context.Background(), Limits{MaxWorkUnits: 1 << 20, MaxAllocationUnits: outputLimit})
	if err != nil {
		panic(err)
	}
	return &utilityHost{ctx: ctx, vm: goja.New()}
}

// textCodecUnits copies UTF-16 units directly so assertions retain supplementary
// pairs and avoid normalizing a result through Go's UTF-8 string conversion.
func textCodecUnits(value goja.String) []uint16 {
	units := make([]uint16, value.Length())
	for i := range units {
		units[i] = value.CharAt(i)
	}
	return units
}

// TestTextCodecEncodeUTF8 preserves scalar boundaries and replaces each lone
// surrogate while recognizing a valid pair without emitting a BOM.
func TestTextCodecEncodeUTF8(t *testing.T) {
	for _, test := range []struct {
		name  string
		input []uint16
		want  string
	}{
		{"empty", nil, ""},
		{"scalar boundaries", []uint16{0, 0x7f, 0x80, 0x7ff, 0x800, 0xd7ff, 0xe000, 0xffff, 0xd800, 0xdc00, 0xdbff, 0xdfff},
			"\x00\x7f\xc2\x80\xdf\xbf\xe0\xa0\x80\xed\x9f\xbf\xee\x80\x80\xef\xbf\xbf\xf0\x90\x80\x80\xf4\x8f\xbf\xbf"},
		{"lone leading", []uint16{0xd800}, "\xef\xbf\xbd"},
		{"lone trailing", []uint16{0xdc00}, "\xef\xbf\xbd"},
		{"separated surrogates", []uint16{0xd800, 'A', 0xdc00}, "\xef\xbf\xbdA\xef\xbf\xbd"},
		{"leading before pair", []uint16{0xd800, 0xd800, 0xdc00}, "\xef\xbf\xbd\xf0\x90\x80\x80"},
		{"trailing before leading", []uint16{0xdc00, 0xd800}, "\xef\xbf\xbd\xef\xbf\xbd"},
		{"authored BOM", []uint16{0xfeff, 'A'}, "\xef\xbb\xbfA"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A native primitive keeps its exact input units, including lone surrogates.
			host := textCodecTestHost(256)
			encoded := host.encodeUTF8(goja.StringFromUTF16(test.input))

			// The encoded bytes and reserved allowance agree exactly.
			require.Equal(t, []byte(test.want), encoded)
			require.Equal(t, len(test.want), executionBudgetFromContext(host.ctx).allocationUnits)
		})
	}
}

// TestTextCodecDecodeUTF8 covers maximal-subpart replacement rather than
// replacing every malformed byte independently, including delimiter recovery.
func TestTextCodecDecodeUTF8(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  []uint16
	}{
		{"empty", "", []uint16{}},
		{"scalar boundaries", "\x00\x7f\xc2\x80\xdf\xbf\xe0\xa0\x80\xed\x9f\xbf\xee\x80\x80\xef\xbf\xbf\xf0\x90\x80\x80\xf4\x8f\xbf\xbf",
			[]uint16{0, 0x7f, 0x80, 0x7ff, 0x800, 0xd7ff, 0xe000, 0xffff, 0xd800, 0xdc00, 0xdbff, 0xdfff}},
		{"truncated two-byte", "\xc2", []uint16{0xfffd}},
		{"truncated three-byte", "\xe2\x82", []uint16{0xfffd}},
		{"truncated four-byte", "\xf0\x9f\x92", []uint16{0xfffd}},
		{"recover ASCII", "\xe2\x82A", []uint16{0xfffd, 'A'}},
		{"recover delimiter", "\xe2\x28\xa1", []uint16{0xfffd, '(', 0xfffd}},
		{"restart leading byte", "\xe2\x80\xc2A", []uint16{0xfffd, 0xfffd, 'A'}},
		{"overlong two-byte", "\xc0\x80", []uint16{0xfffd, 0xfffd}},
		{"overlong three-byte", "\xe0\x80\x80", []uint16{0xfffd, 0xfffd, 0xfffd}},
		{"overlong four-byte", "\xf0\x80\x80\x80", []uint16{0xfffd, 0xfffd, 0xfffd, 0xfffd}},
		{"encoded surrogate", "\xed\xa0\x80", []uint16{0xfffd, 0xfffd, 0xfffd}},
		{"above Unicode maximum", "\xf4\x90\x80\x80", []uint16{0xfffd, 0xfffd, 0xfffd, 0xfffd}},
		{"isolated continuation", "\x80\xbf", []uint16{0xfffd, 0xfffd}},
		{"invalid leading bytes", "\xfe\xff", []uint16{0xfffd, 0xfffd}},
		{"valid replacement scalar", "\xef\xbf\xbd", []uint16{0xfffd}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Default decoding replaces malformed subparts and preserves following input.
			host := textCodecTestHost(256)
			decoded := host.decodeUTF8([]byte(test.input), false, false)

			// Output is charged in UTF-16 units, including both halves of astral pairs.
			require.Equal(t, test.want, textCodecUnits(decoded))
			require.Equal(t, len(test.want), executionBudgetFromContext(host.ctx).allocationUnits)
		})
	}
}

// TestTextCodecDecodeUTF8BOM applies BOM policy independently to each complete
// call, stripping only the first initial marker when ignoreBOM is false.
func TestTextCodecDecodeUTF8BOM(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     string
		ignoreBOM bool
		want      []uint16
	}{
		{"strip initial", "\xef\xbb\xbfA", false, []uint16{'A'}},
		{"strip one marker", "\xef\xbb\xbf\xef\xbb\xbf", false, []uint16{0xfeff}},
		{"preserve interior", "A\xef\xbb\xbf", false, []uint16{'A', 0xfeff}},
		{"preserve after malformed", "\xff\xef\xbb\xbf", false, []uint16{0xfffd, 0xfeff}},
		{"ignore detection", "\xef\xbb\xbfA", true, []uint16{0xfeff, 'A'}},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := textCodecTestHost(256)
			for range 2 {
				// Reusing a decoder must not carry BOM-detection state into the next call.
				decoded := host.decodeUTF8([]byte(test.input), false, test.ignoreBOM)
				require.Equal(t, test.want, textCodecUnits(decoded))
			}
		})
	}
}

// TestTextCodecFatalDecodeReservesNoOutput validates the full input before an
// allocation is admitted, even when valid text precedes the malformed bytes.
func TestTextCodecFatalDecodeReservesNoOutput(t *testing.T) {
	host := textCodecTestHost(1)
	var result goja.String

	// Fatal validation wins without spending output on the valid prefix.
	exception := host.vm.Try(func() {
		result = host.decodeUTF8([]byte("private prefix\xe2\x82"), true, false)
	})
	require.NotNil(t, exception)
	require.Nil(t, result)
	require.Nil(t, executionBudgetFromContext(host.ctx).err())
	require.Zero(t, executionBudgetFromContext(host.ctx).allocationUnits)

	// A following valid decode can still use the entire output allowance.
	result = host.decodeUTF8([]byte("A"), true, false)
	require.Equal(t, []uint16{'A'}, textCodecUnits(result))
	require.Equal(t, 1, executionBudgetFromContext(host.ctx).allocationUnits)
}

// TestTextCodecFatalDecodeAcceptsReplacementScalar distinguishes a well-formed
// U+FFFD sequence from the decoder's replacement for malformed input.
func TestTextCodecFatalDecodeAcceptsReplacementScalar(t *testing.T) {
	host := textCodecTestHost(1)

	// Fatal mode accepts an authored replacement scalar encoded with valid UTF-8.
	result := host.decodeUTF8([]byte("\xef\xbf\xbd"), true, false)
	require.Equal(t, []uint16{0xfffd}, textCodecUnits(result))
	require.Equal(t, 1, executionBudgetFromContext(host.ctx).allocationUnits)
}

// TestTextCodecUTF8OutputReservation rejects an over-budget result before its
// buffer is allocated, retaining the utility host's sticky failure classification.
func TestTextCodecUTF8OutputReservation(t *testing.T) {
	for _, encode := range []bool{false, true} {
		t.Run(map[bool]string{false: "decode", true: "encode"}[encode], func(t *testing.T) {
			host := textCodecTestHost(1)
			var encoded []byte
			var decoded goja.String

			// One output unit cannot fit a supplementary character or its UTF-8 bytes.
			exception := host.vm.Try(func() {
				if encode {
					encoded = host.encodeUTF8(goja.StringFromUTF16([]uint16{0xd800, 0xdc00}))
				} else {
					decoded = host.decodeUTF8([]byte("\xf0\x90\x80\x80"), false, false)
				}
			})
			require.NotNil(t, exception)
			require.Nil(t, encoded)
			require.Nil(t, decoded)
			require.ErrorIs(t, executionBudgetFromContext(host.ctx).err(), ErrLimitExceeded)
			require.Zero(t, executionBudgetFromContext(host.ctx).allocationUnits)
		})
	}
}
