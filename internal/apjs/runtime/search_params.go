package runtime

import (
	"strconv"

	"github.com/dop251/goja"
)

// searchParamsConstructorFactory preserves JavaScript's new/subclass behavior
// while keeping the native initializer private. No authored code is compiled.
var searchParamsConstructorFactory = goja.MustCompile("", `(function (initialize) {
	return class URLSearchParams {
		constructor(init) { initialize(this, init); }
	};
})`, true)

// searchParams owns one VM's branded query objects. Execution work bounds entries;
// shared output allowances bound their retained strings and tuple storage.
type searchParams struct {
	host      *utilityHost                        // Owns the execution's shared counters and deadline.
	instances map[*goja.Object]*searchParamsState // Private state, never reflected into JavaScript.
}

// searchParamsState retains ordered duplicate pairs. scanUnits caches their
// UTF-16 lengths plus one per tuple, including tuples with two empty strings.
type searchParamsState struct {
	pairs     []searchParam // Native immutable strings; mutations replace tuple references.
	scanUnits int           // Work charged before a traversal, maintained after mutations.
}

// newSearchParams creates isolated receiver branding before authored code runs.
func newSearchParams(host *utilityHost) *searchParams {
	return &searchParams{
		host:      host,
		instances: make(map[*goja.Object]*searchParamsState),
	}
}

// install exposes a protected constructor, methods, and read-only size getter.
// The string-only surface deliberately omits coercion, iterators, and callbacks.
func (p *searchParams) install() error {
	vm := p.host.vm

	value, err := vm.RunProgram(searchParamsConstructorFactory)
	if err != nil {
		return EvaluationError(err)
	}

	factory, _ := goja.AssertFunction(value)

	value, err = factory(goja.Undefined(), vm.ToValue(p.initialize))
	if err != nil {
		return EvaluationError(err)
	}

	constructor := value.ToObject(vm)
	prototype := constructor.Get("prototype").ToObject(vm)

	for name, method := range map[string]func(goja.FunctionCall) goja.Value{
		"append":   p.append,
		"set":      p.set,
		"delete":   p.delete,
		"get":      p.get,
		"getAll":   p.getAll,
		"has":      p.has,
		"sort":     p.sort,
		"toString": p.toString,
	} {
		if err := prototype.DefineDataProperty(
			name,
			vm.ToValue(method),
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
	}

	if err := prototype.DefineAccessorProperty(
		"size",
		vm.ToValue(p.size),
		goja.Undefined(),
		goja.FLAG_FALSE, // configurable
		goja.FLAG_FALSE, // enumerable
	); err != nil {
		return ErrInvalidInput
	}

	for _, property := range []struct {
		object *goja.Object
		name   string
		value  goja.Value
	}{
		{constructor, "name", vm.ToValue("URLSearchParams")},
		{constructor, "prototype", prototype},
		{prototype, "constructor", constructor},
		{vm.GlobalObject(), "URLSearchParams", constructor},
	} {
		if err := property.object.DefineDataProperty(
			property.name,
			property.value,
			goja.FLAG_FALSE, // writable
			goja.FLAG_FALSE, // configurable
			goja.FLAG_FALSE, // enumerable
		); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

// initialize parses an optional primitive query string into detached native
// state. Failed construction consumes admitted work but retains no instance.
func (p *searchParams) initialize(call goja.FunctionCall) goja.Value {
	p.host.beginCall()

	receiver, ok := call.Argument(0).(*goja.Object)
	if !ok {
		p.invalidInput()
	}

	input := call.Argument(1)
	if goja.IsUndefined(input) {
		input = p.host.vm.ToValue("")
	}

	text, ok := input.(goja.String)
	if !ok {
		p.invalidInput()
	}

	p.host.consumeInput(text.Length())
	state := &searchParamsState{
		pairs: p.host.parseSearchParams(text),
	}
	p.recount(state)
	p.host.reserveOutput(1) // Retained native receiver metadata.
	p.instances[receiver] = state

	return goja.Undefined()
}

// begin charges every native entry, then rejects fake/proxy receivers without
// reading their properties or invoking authored traps.
func (p *searchParams) begin(call goja.FunctionCall) *searchParamsState {
	p.host.beginCall()

	receiver, ok := call.This.(*goja.Object)
	if !ok {
		p.invalidInput()
	}

	state, ok := p.instances[receiver]
	if !ok {
		p.invalidInput()
	}

	return state
}

// argument admits a primitive string and applies scalar-string normalization.
// The temporary UTF-8 bytes and normalized UTF-16 string both consume output.
func (p *searchParams) argument(value goja.Value) goja.String {
	text, ok := value.(goja.String)
	if !ok {
		p.invalidInput()
	}

	p.host.consumeInput(text.Length())

	return p.host.searchParamString(text)
}

// append adds a pair at the end without scanning existing tuples.
func (p *searchParams) append(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	pair := searchParam{
		name:  p.argument(call.Argument(0)),
		value: p.argument(call.Argument(1)),
	}
	p.host.reserveOutput(1)
	p.addUnits(state, pair)

	state.pairs = append(state.pairs, pair)

	return goja.Undefined()
}

// set replaces the first matching pair, removes later duplicates, and preserves
// the position of unrelated pairs. Missing names append a new tuple.
func (p *searchParams) set(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	pair := searchParam{
		name:  p.argument(call.Argument(0)),
		value: p.argument(call.Argument(1)),
	}

	p.host.consumeInput(state.scanUnits)
	retained, found := 0, false

	for i, existing := range state.pairs {
		p.poll(i)
		if p.compare(existing.name, pair.name) == 0 {
			if found {
				continue
			}
			existing, found = pair, true
		}
		state.pairs[retained] = existing
		retained++
	}

	clear(state.pairs[retained:])

	state.pairs = state.pairs[:retained]
	if !found {
		p.host.reserveOutput(1)
		state.pairs = append(state.pairs, pair)
	}

	p.recount(state)

	return goja.Undefined()
}

// filterArguments normalizes the name and optional value used by has/delete.
// An explicitly undefined value has the same meaning as omitting it.
func (p *searchParams) filterArguments(
	call goja.FunctionCall,
) (goja.String, goja.String) {
	name := p.argument(call.Argument(0))

	var value goja.String
	if !goja.IsUndefined(call.Argument(1)) {
		value = p.argument(call.Argument(1))
	}

	return name, value
}

// delete removes matching names, optionally restricted to a matching value.
func (p *searchParams) delete(call goja.FunctionCall) goja.Value {
	state := p.begin(call)

	name, value := p.filterArguments(call)
	p.host.consumeInput(state.scanUnits)
	retained := 0

	for i, pair := range state.pairs {
		p.poll(i)
		if p.matches(pair, name, value) {
			continue
		}
		state.pairs[retained] = pair
		retained++
	}

	// Release removed references even when the backing slice is retained.
	clear(state.pairs[retained:])
	state.pairs = state.pairs[:retained]
	p.recount(state)

	return goja.Undefined()
}

// get returns the first matching value or null. Returned string units are
// charged even though immutable native strings can safely share storage.
func (p *searchParams) get(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	name := p.argument(call.Argument(0))
	p.host.consumeInput(state.scanUnits)
	for i, pair := range state.pairs {
		p.poll(i)
		if p.compare(pair.name, name) == 0 {
			p.host.reserveOutput(pair.value.Length())
			return pair.value
		}
	}
	return goja.Null()
}

// getAll reserves a native array's slots and strings before allocating it.
// Defining own data properties bypasses authored Array.prototype setters.
func (p *searchParams) getAll(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	name := p.argument(call.Argument(0))
	p.host.consumeInput(state.scanUnits)
	for i, pair := range state.pairs {
		p.poll(i)
		if p.compare(pair.name, name) == 0 {
			p.host.reserveOutput(1)
			p.host.reserveOutput(pair.value.Length())
		}
	}
	array := p.host.vm.NewArray()
	index := 0
	for i, pair := range state.pairs {
		p.poll(i)
		if p.compare(pair.name, name) != 0 {
			continue
		}
		if err := array.DefineDataProperty(strconv.Itoa(index), pair.value, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
			// The fresh array has no authored descriptors; never expose an
			// internal diagnostic or disguise an uncatchable VM failure.
			switch err.(type) {
			case *goja.InterruptedError, *goja.StackOverflowError:
				panic(err)
			default:
				p.invalidInput()
			}
		}
		index++
	}
	p.host.checkContext()
	return array
}

// has tests whether at least one pair matches the normalized name/value filter.
func (p *searchParams) has(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	name, value := p.filterArguments(call)
	p.host.consumeInput(state.scanUnits)
	for i, pair := range state.pairs {
		p.poll(i)
		if p.matches(pair, name, value) {
			return p.host.vm.ToValue(true)
		}
	}
	return p.host.vm.ToValue(false)
}

// sort stably orders names by UTF-16 code units with a bottom-up merge sort.
// Both tuple moves and name comparisons are charged, and the scratch slots are
// reserved before allocation. Explicit loops poll even while copying a tail
// with no remaining comparisons; native library sorts cannot provide that hook.
func (p *searchParams) sort(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	p.host.consumeInput(state.scanUnits)
	length := len(state.pairs)
	if length < 2 {
		return goja.Undefined()
	}
	p.host.reserveOutput(length)
	if length > int(^uint(0)>>1)/32 {
		p.host.failLimit()
	}
	source, target := state.pairs, make([]searchParam, length)
	for width := 1; width < length; {
		for start := 0; start < length; {
			middle := start + min(width, length-start)
			end := middle + min(width, length-middle)
			left, right := start, middle
			for next := start; next < end; next++ {
				p.poll(next)
				p.host.consumeInput(1)
				useLeft := right == end
				if left < middle && right < end {
					p.host.consumeInput(source[left].name.Length())
					p.host.consumeInput(source[right].name.Length())
					useLeft = p.compare(source[left].name, source[right].name) <= 0
				}
				if useLeft {
					target[next] = source[left]
					left++
				} else {
					target[next] = source[right]
					right++
				}
			}
			start = end
		}
		source, target = target, source
		// Stop after the final merge without overflowing width * 2.
		if width >= length-width {
			break
		}
		width *= 2
	}
	state.pairs = source
	p.host.checkContext()
	return goja.Undefined()
}

// size returns the tuple count without scanning or exposing native state.
func (p *searchParams) size(call goja.FunctionCall) goja.Value {
	return p.host.vm.ToValue(len(p.begin(call).pairs))
}

// toString emits form-encoded query text without a leading question mark.
func (p *searchParams) toString(call goja.FunctionCall) goja.Value {
	state := p.begin(call)
	p.host.consumeInput(state.scanUnits)
	return p.host.serializeSearchParams(state.pairs)
}

// matches compares normalized strings without coercion or authored callbacks.
func (p *searchParams) matches(pair searchParam, name, value goja.String) bool {
	return p.compare(pair.name, name) == 0 && (value == nil || p.compare(pair.value, value) == 0)
}

// compare lexicographically compares UTF-16 units, polling during long shared
// prefixes. Its caller has already admitted the strings' comparison work.
func (p *searchParams) compare(a, b goja.String) int {
	for i := 0; i < min(a.Length(), b.Length()); i++ {
		p.poll(i)
		if left, right := a.CharAt(i), b.CharAt(i); left != right {
			return int(left) - int(right)
		}
	}
	p.host.checkContext()
	return a.Length() - b.Length()
}

// recount rebuilds cached traversal cost after a bounded mutation or parsing.
func (p *searchParams) recount(state *searchParamsState) {
	state.scanUnits = 0
	for i, pair := range state.pairs {
		p.poll(i)
		p.addUnits(state, pair)
	}
	p.host.checkContext()
}

// addUnits includes empty-tuple metadata and prevents cached-cost overflow.
func (p *searchParams) addUnits(state *searchParamsState, pair searchParam) {
	for _, size := range []int{1, pair.name.Length(), pair.value.Length()} {
		if size > int(^uint(0)>>1)-state.scanUnits {
			p.host.failLimit()
		}
		state.scanUnits += size
	}
}

// poll bounds native iterations between cancellation checks, including empties.
func (p *searchParams) poll(index int) {
	if index&base64PollMask == 0 {
		p.host.checkContext()
	}
}

// invalidInput throws fixed catchable text without inspecting private values.
func (p *searchParams) invalidInput() {
	panic(p.host.vm.NewTypeError("Invalid URLSearchParams input"))
}
