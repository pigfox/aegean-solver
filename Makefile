# Recipe lines below are indented with TABS. Make requires it — spaces produce
# "missing separator" on every target.
#
# -buildvcs=false on every go invocation. The build stamps version-control
# metadata into a binary by default, which means the toolchain reads the
# repository state during a test run and can fail for reasons that have nothing
# to do with the code — a detached head, a stale index, a directory the build
# user cannot stat. A test gate must depend on the source and nothing else.

.PHONY: test cover clean

test:
	go test -buildvcs=false -race -shuffle=on ./...
	go vet -buildvcs=false ./...
	@test -z "$$(gofmt -l .)" || { echo "gofmt needs to run on:"; gofmt -l .; exit 1; }

# Coverage is 100% of statements across every package and is expected to stay
# there. Each statement is either an algorithm this repository exists to get
# right or a refusal that protects one, so there is nothing here that would be
# honest to exclude — and there is no exclusion list.
#
# THE SECOND RECIPE LINE IS THE POINT. Printing a percentage and exiting zero is
# a log line, not a gate: it reports the shortfall to a terminal nobody is
# reading and then succeeds. This fails the build on any line below 100% and
# names every one of them.
cover:
	go test -buildvcs=false -covermode=set -coverprofile=cover.out ./...
	@go tool cover -func=cover.out | tail -1
	@go tool cover -func=cover.out | awk '$$NF != "100.0%" { print "UNCOVERED: " $$0; bad = 1 } END { if (bad) { print "coverage gate: statements below 100%"; exit 1 } }'

clean:
	rm -f cover.out
