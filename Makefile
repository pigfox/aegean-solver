# Recipe lines below are indented with TABS. Make requires it — spaces produce
# "missing separator" on every target.

.PHONY: test cover clean

test:
	go test -race -shuffle=on ./...
	go vet ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needs to run on:"; gofmt -l .; exit 1; }

# Coverage is 100% of statements and is expected to stay there. There is one
# package and every statement in it is either an algorithm this repository
# exists to get right or a refusal that protects one, so there is nothing here
# that would be honest to exclude.
cover:
	go test -covermode=set -coverprofile=cover.out ./...
	go tool cover -func=cover.out | tail -1

clean:
	rm -f cover.out
