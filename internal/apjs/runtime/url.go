package runtime

import (
	"github.com/dop251/goja"
	whatwgurl "github.com/nlnwa/whatwg-url/url"
)

// urlConstructorFactory supplies ordinary new/subclass semantics without
// exposing the native initializer or compiling any authored source.
var urlConstructorFactory = goja.MustCompile("", `(function (initialize) {
	return class URL {
		constructor(input, base) { initialize(this, input, base); }
	};
})`, true)

// urlHost owns this VM's immutable HTTP(S) URL values. A private receiver map
// prevents forged objects and proxies from invoking native methods or getters.
// Parser results never enter JavaScript as reflected Go objects.
type urlHost struct {
	host      *utilityHost                       // Execution accounting, cancellation, and owner VM.
	parser    whatwgurl.Parser                   // Owner-only parser with default URL Standard behavior.
	instances map[*goja.Object]map[string]string // Cached serialized components, retained until VM disposal.
}

// newURLHost creates independent receiver state. Diagnostic collection stays
// disabled so malformed inputs cannot retain a list of private parser errors.
func newURLHost(host *utilityHost) *urlHost {
	return &urlHost{
		host: host, parser: whatwgurl.NewParser(),
		instances: make(map[*goja.Object]map[string]string),
	}
}

// install exposes a protected URL constructor, immutable component getters,
// and explicit serializers. Setters and linked searchParams are not supported.
func (u *urlHost) install() error {
	vm := u.host.vm
	value, err := vm.RunProgram(urlConstructorFactory)
	if err != nil {
		return EvaluationError(err)
	}
	factory, _ := goja.AssertFunction(value)
	value, err = factory(goja.Undefined(), vm.ToValue(u.initialize))
	if err != nil {
		return EvaluationError(err)
	}
	constructor := value.ToObject(vm)
	prototype := constructor.Get("prototype").ToObject(vm)
	for _, name := range []string{
		"href", "origin", "protocol", "username", "password", "host", "hostname",
		"port", "pathname", "search", "hash",
	} {
		if err := prototype.DefineAccessorProperty(name, vm.ToValue(u.getter(name)), goja.Undefined(),
			goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return ErrInvalidInput
		}
	}
	for _, name := range []string{"toString", "toJSON"} {
		if err := prototype.DefineDataProperty(name, vm.ToValue(u.getter("href")),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return ErrInvalidInput
		}
	}
	for _, property := range []struct {
		object *goja.Object
		name   string
		value  goja.Value
	}{
		{constructor, "name", vm.ToValue("URL")},
		{constructor, "prototype", prototype},
		{prototype, "constructor", constructor},
		{vm.GlobalObject(), "URL", constructor},
	} {
		if err := property.object.DefineDataProperty(property.name, property.value,
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return ErrInvalidInput
		}
	}
	return nil
}

// initialize normalizes primitive strings, validates an optional HTTP(S) base,
// and caches the resolved URL's components. Even an absolute input validates a
// supplied base. No partial receiver is branded when parsing or admission fails.
func (u *urlHost) initialize(call goja.FunctionCall) goja.Value {
	u.host.beginCall()
	receiver, ok := call.Argument(0).(*goja.Object)
	if !ok {
		u.invalidInput()
	}
	input := u.argument(call.Argument(1))
	var base *whatwgurl.Url
	baseLength := 0
	if value := call.Argument(2); !goja.IsUndefined(value) {
		baseText := u.argument(value)
		u.admitNative(len(baseText))
		var err error
		base, err = u.parser.Parse(baseText)
		u.checkParsed(base, err)
		// Serialization can copy path segments repeatedly. The admission above
		// covers this native pass; its actual size bounds the later base clone.
		baseLength = len(base.Href(false))
		u.host.checkContext()
	}
	if len(input) > int(^uint(0)>>1)-baseLength {
		u.host.failLimit()
	}
	u.admitNative(len(input) + baseLength)
	parsed, err := u.parser.BasicParser(input, base, nil, whatwgurl.NoState)
	u.checkParsed(parsed, err)

	// Cache once under the parser's conservative admission. Subsequent reads
	// charge only the component they return, never reserialize the native URL.
	fields := map[string]string{
		"href": parsed.Href(false), "protocol": parsed.Protocol(),
		"username": parsed.Username(), "password": parsed.Password(),
		"host": parsed.Host(), "hostname": parsed.Hostname(), "port": parsed.Port(),
		"pathname": parsed.Pathname(), "search": parsed.Search(), "hash": parsed.Hash(),
	}
	fields["origin"] = fields["protocol"] + "//" + fields["host"]
	u.host.reserveOutput(1 + len(fields)) // Retained receiver and component slots.
	for _, text := range fields {
		u.host.consumeInput(len(text))
		u.host.reserveOutput(len(text))
	}
	u.host.checkContext()
	u.instances[receiver] = fields
	return goja.Undefined()
}

// getter returns a branded native accessor or serializer. Cached UTF-8 byte
// lengths conservatively bound the JavaScript strings' UTF-16 storage.
func (u *urlHost) getter(name string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		u.host.beginCall()
		receiver, ok := call.This.(*goja.Object)
		if !ok {
			u.invalidInput()
		}
		fields, ok := u.instances[receiver]
		if !ok {
			u.invalidInput()
		}
		text := fields[name]
		u.host.consumeInput(len(text))
		u.host.reserveOutput(len(text))
		value := u.host.vm.ToValue(text)
		u.host.checkContext()
		return value
	}
}

// invalidInput throws a fixed catchable error, never a parser diagnostic, URL,
// credentials, or authored coercion result.
func (u *urlHost) invalidInput() {
	panic(u.host.vm.NewTypeError("Invalid URL input"))
}
