# Tool execution contracts

This package defines the shared `Executor` interface and `Invocation` input used
by the [HTTP](httpexec/README.md) and [JavaScript](jsexec/README.md) executors.
Core selects and compiles the appropriate executor, then calls it through the
same interface. Executor implementations and their limits remain in their own
packages.

`Invocation.Arguments` contains caller input, such as `{"id":"customer-42"}`.
`Invocation.Snapshot` contains the admitted connection configuration, metadata,
and captured Tool revision. Executors validate the arguments and replace
`Snapshot.Params`; callers should leave it unset. The optional deadline includes
time already spent preparing the connection and compiling the Tool.

`Invocation.Logger` carries the configured service sink and captured scalar IDs
for JavaScript HTTP-worker diagnostics. It does not enable authored console
logging. The declarative HTTP executor does not currently use this optional
logger; its configured client retains the normal proxy logging behavior.

Admission must associate the compiled program, snapshot, and configured HTTP
client with the same authorized capture. The client owns credentials and request
attribution. This package does not discover connections, check permissions, or
define public API request types.
