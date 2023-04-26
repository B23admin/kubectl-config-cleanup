build:
  #!/usr/bin/env bash
  go build -o kubectl-config_cleanup main.go

test:
  #!/usr/bin/env bash
  go test ./cleanup

package:
  #!/usr/bin/env bash
  goreleaser --snapshot --skip-publish --clean
