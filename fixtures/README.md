# Adapter Fixtures

`representative.md` covers a read-only reviewer with permissions and hook
registration. `golden/` contains the expected native output for all five
adapters; `go test ./...` compares the renderer against these files.
