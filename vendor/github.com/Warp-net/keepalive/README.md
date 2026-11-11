# keepalive
```shell
go get github.com/Warp-net/keepalive
```
Requires Go 1.25+. MIT license.

## Integration

```go
import (
    "context"
    "time"

    power "keepalive/power"
)

// After starting the node:
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
hold, err := power.New().Acquire(ctx, power.Options{
    KeepDisplay: false,
    Reason: "serving paired devices",
})
cancel()
if err != nil {
    // Sleep inhibition failed. Report the error in the logs or UI.
    return err
}

// Keep hold for the entire lifetime of the node.
// When stopping the node, call:
err = hold.Release()
```

Do not use `defer hold.Release()` in a short startup function: it would release the inhibitor as soon as the function returns. Use `defer` in a function that runs for the entire lifetime of the node, such as `main`.

For a complete runnable example, use `go run ./examples/hold` (press Ctrl+C to exit). It acquires a real sleep inhibitor; the display can still turn off and the screen can still lock.

## API Contract

- `New().Acquire(ctx, Options)` acquires a sleep inhibitor.
- `KeepDisplay: false` keeps only the system awake; `true` also prevents the display from turning off and may prevent the screen from locking.
- The context controls acquisition only. Cancelling it after a successful `Acquire` **does not release** the inhibitor: call `Release()` when stopping the node.
- `Hold.Release()` frees resources and is safe to call repeatedly.
- `Hold.Describe()` reports the mechanisms actually in use.
- If the hold implements `Watcher`, its `Lost()` channel reports a lost D-Bus connection or a helper process exiting.
  The application must handle this error and acquire a new inhibitor if needed; the library does not recover automatically.
- `Mechanisms()` provides diagnostics, and `Hint(err)` returns a troubleshooting hint.

## Platforms

| Platform | Upstream implementation |
| --- | --- |
| Windows | PowerCreateRequest / PowerSetRequest / PowerClearRequest |
| macOS with cgo | IOKit power assertions |
| macOS without cgo | Built-in caffeinate utility |
| Linux | logind over D-Bus; systemd-inhibit fallback; available GNOME / ScreenSaver / PowerManagement session APIs, depending on KeepDisplay |
| Other platforms | Explicit unsupported-platform error |

On Linux, the choice depends on available services, not the distribution name. With `KeepDisplay: false`, 
the library intentionally avoids ScreenSaver and PowerManagement because they may also prevent screen locking or display sleep. Acquisition may therefore fail in environments without logind/systemd-inhibit and without the GNOME suspend API. The library does not guarantee support for every Linux environment.

Sleep inhibition does not keep a GUI process running after an actual logout from the system session, 
nor does it guarantee continued operation during forced sleep, shutdown, or restrictions imposed by administrator policies. The OS releases held resources when the process exits; helper processes are also tied to its lifetime.

## Testing

```sh
go test -race -short ./...
go vet ./...
```

For native Windows/macOS checks, run `go test ./...` on the target OS without `-short`: these tests acquire 
real power assertions.

For Linux testing with separate D-Bus buses and python-dbusmock:

```sh
sh test/docker/power/run.sh -race
```

A running Docker daemon is required. The tests verify real D-Bus calls, the logind file descriptor,
and the systemd-inhibit lifecycle against mocked services.