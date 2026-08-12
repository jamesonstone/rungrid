# Process Compose v1.120 macOS missing-socket client spin

Status: sanitized upstream report draft; do not publish without explicit
authorization.

## Summary

On macOS, a Process Compose v1.120 finite CLI client remained alive and consumed
approximately ten logical cores after the daemon and recorded Unix socket had
disappeared. The observed client was invoked as `process get <service> --output
json` by an owning development-session loop. It ran for more than 11 hours
instead of returning a connection error.

## Environment

- macOS on Apple Silicon
- Process Compose v1.120.0
- Unix-socket mode (`-U -u <socket>`)
- detached daemon and a separate finite state-query client

No repository names, developer paths, socket paths, process IDs, environment
values, commands containing user arguments, or application output from the
original incident are included here.

## Candidate reproduction

1. Create a temporary directory and a minimal Process Compose project with one
   long-running process.
2. Start Process Compose detached with Unix-socket mode and wait for the socket
   to answer.
3. In a separate process group, repeatedly invoke the finite client:
   `process-compose -U -u runtime.sock process get example --output json`.
4. Concurrently ask the daemon to shut down, then remove only the socket after
   daemon exit has been verified.
5. Assert that every finite client exits promptly with a non-zero connection
   error. Record PID, start time, elapsed time, and CPU only; do not collect
   commands, environment, or service output.
6. Repeat the shutdown at different client phases: before connect, during
   connect, after request write, and while reading the response.

The original symptom has not yet been reproduced by this sanitized harness.
That distinction should remain explicit in any upstream report.

## Expected behavior

The finite CLI query exits within a bounded interval when its Unix socket is
missing, replaced, closes, or stops answering. It must not retry indefinitely
or enter a busy loop.

## Actual behavior

One v1.120.0 client outlived the daemon/socket and consumed roughly 1,064
percent process CPU for more than 11 hours. The parent tool has replaced finite
CLI state/lifecycle calls with deadline-bound HTTP over the Unix socket and
isolates the remaining streaming clients, so this downstream mitigation no
longer supplies a minimal upstream reproduction by itself.

## Requested upstream investigation

- Audit Unix-socket reconnect/error paths used by finite commands for retry
  loops without delay or cancellation.
- Confirm that `process get`, `list`, `project state`, readiness waits, and
  other non-streaming commands return after daemon/socket loss.
- Add a macOS regression test that removes or replaces the socket during each
  finite request phase and asserts bounded exit plus low CPU.

## Evidence to attach only after sanitization

- exact `process-compose version` output;
- a minimal temporary-project reproducer;
- bounded `sample` or equivalent stack evidence from the spinning client;
- CPU and elapsed-time observations without application output; and
- confirmation that the PID/start identity belongs to the finite client, not
  the daemon or managed application.
