set shell := ["bash", "-euo", "pipefail", "-c"]

build:
  go build -o kubectl-config_cleanup main.go

test:
  go test ./cleanup

package:
  goreleaser --snapshot --skip-publish --clean

release tag_name:
  git tag -a {{ tag_name }}
  git push origin {{ tag_name }}
