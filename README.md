# go-shift-scheduling-grpc

A Go backend service for scheduling employee work shifts, built with **gRPC** and **Protocol Buffers**.

Clients can create shifts, look them up and assign employees. The service enforces real scheduling rules, such as preventing an employee from being booked on overlapping shifts.

**Tech:** Go · gRPC · Protocol Buffers · Go standard `testing` package

## What this project demonstrates

- **API-first design:** the API is defined once in a versioned `.proto` contract (`shift.v1`), and Go client/server code is generated from it.
- **Clean layering:** business rules live in a package with no gRPC dependency. A thin transport layer translates between gRPC and the domain.
- **Correct error handling:** domain errors map to meaningful gRPC status codes (`InvalidArgument`, `NotFound`, `FailedPrecondition`). Unexpected errors return `Internal` without leaking internal details.
- **Concurrency safety:** shared state is protected with a `sync.RWMutex`, and assignments are serialized so two simultaneous requests cannot double-book an employee.
- **Thorough testing:** table-driven unit tests for the business rules, plus API tests that run a real gRPC client and server in memory.
- **Operational basics:** configurable port, gRPC reflection for tooling, and graceful shutdown on Ctrl+C.

## Business rules

- A shift must have a start and end time, and the start must be before the end.
- An employee cannot be assigned two overlapping shifts.
- Shifts are half-open intervals `[start, end)`, so back-to-back shifts (one ends at 17:00, the next starts at 17:00) are allowed.
- A shift already assigned to someone else cannot be taken by another employee.
- Different employees may work at the same time.

## API

Service `shift.v1.ShiftService`, defined in [`api/proto/shift/v1/shift.proto`](api/proto/shift/v1/shift.proto).

| RPC | Purpose | Error codes |
| --- | --- | --- |
| `CreateShift` | Create an unassigned shift | `InvalidArgument` |
| `GetShift` | Get a shift by ID | `InvalidArgument`, `NotFound` |
| `AssignEmployee` | Assign an employee to a shift | `InvalidArgument`, `NotFound`, `FailedPrecondition` |

## Architecture

```text
gRPC client
    │
    ▼
internal/grpcserver    validate request, convert protobuf ↔ domain, map errors to status codes
    │
    ▼
internal/scheduling    business rules (no gRPC dependency)
    │
    ▼
MemoryRepository       in-memory storage, safe for concurrent use
```

```text
api/proto/      API contract (.proto)
gen/            generated Go code
cmd/server/     entry point
internal/       application code: grpcserver (transport), scheduling (domain)
```

### Key design decisions

- **`FailedPrecondition` for scheduling conflicts:** the request is valid, but the current state does not allow it. `AlreadyExists` is meant for creating something that already exists.
- **In-memory storage behind a `Repository` interface:** this keeps the project focused and dependency-free. A database could replace it without touching the business rules.
- **Generated code is committed**, so the project builds with only Go installed.

## Run locally

Requires **Go 1.25+**.

```powershell
go test ./...          # run all tests
go run ./cmd/server    # start the server on localhost:50051 (Ctrl+C to stop)
```

**Note:** data is kept in memory, so all shifts are lost when the server stops.

<details>
<summary>Try the API with grpcurl (optional)</summary>

With the server running, in a second PowerShell window:

```powershell
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Send piped text to grpcurl as plain UTF-8 (avoids a PowerShell encoding issue)
$OutputEncoding = [Console]::InputEncoding = [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding $false

# Create a shift
$id = ('{"start_time": "2026-11-02T09:00:00Z", "end_time": "2026-11-02T17:00:00Z"}' |
  grpcurl -plaintext -d '@' localhost:50051 shift.v1.ShiftService/CreateShift |
  Out-String | ConvertFrom-Json).shift.id

# Get it
@{ id = $id } | ConvertTo-Json -Compress |
  grpcurl -plaintext -d '@' localhost:50051 shift.v1.ShiftService/GetShift

# Assign an employee
@{ shift_id = $id; employee_id = "emp-123" } | ConvertTo-Json -Compress |
  grpcurl -plaintext -d '@' localhost:50051 shift.v1.ShiftService/AssignEmployee
```

If `grpcurl` is not recognized, add Go's bin folder to the PATH for this window: `$env:Path += ";$(go env GOPATH)\bin"`.

</details>

<details>
<summary>Regenerate code after changing the .proto (contributors only)</summary>

Requires `protoc`, plus the plugins installed with:

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Then run:

```powershell
protoc --proto_path=api/proto `
  --go_out=. --go_opt=module=github.com/dishamistry/go-shift-scheduling-grpc `
  --go-grpc_out=. --go-grpc_opt=module=github.com/dishamistry/go-shift-scheduling-grpc `
  api/proto/shift/v1/shift.proto
```

</details>

## Limitations

- In-memory storage only; there is no database.
- No authentication or TLS. The server listens on localhost only.
