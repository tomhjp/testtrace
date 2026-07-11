# testtrace

Visualise your go tests' timings using perfetto traces.

## Example usage

```
go install github.com/tomhjp/testtrace/cmd/testtrace@latest
go test -json ./... | testtrace > trace.json
```

Then open trace.json in [perfetto](https://ui.perfetto.dev/) to view the timing
of all tests that ran.
