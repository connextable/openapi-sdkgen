set quiet := true
set no-cd := true

[private]
default:
    @just --list agent

check:
    ./scripts/agent/check

fmt:
    ./scripts/agent/fmt

fmt-check:
    ./scripts/agent/fmt-check

vet:
    ./scripts/agent/vet

test:
    ./scripts/agent/test

build:
    ./scripts/agent/build

mod-tidy:
    ./scripts/agent/mod-tidy

mod-tidy-check:
    ./scripts/agent/mod-tidy-check

mod-verify:
    ./scripts/agent/mod-verify

generate input output target='typescript':
    ./scripts/agent/generate {{quote(input)}} {{quote(output)}} {{quote(target)}}

conformance:
    ./scripts/agent/conformance

ts-install:
    ./scripts/agent/ts-install

ts-fmt:
    ./scripts/agent/ts-fmt

ts-fmt-check:
    ./scripts/agent/ts-fmt-check

ts-lint:
    ./scripts/agent/ts-lint

ts-typecheck:
    ./scripts/agent/ts-typecheck

ts-test:
    ./scripts/agent/ts-test

release-check:
    ./scripts/agent/release-check
