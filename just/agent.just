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

coverage:
    ./scripts/agent/coverage

ts-lock:
    ./scripts/agent/ts-lock

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

example-todo:
    ./scripts/agent/example-todo

example-advanced:
    ./scripts/agent/example-advanced

example-capabilities:
    ./scripts/agent/example-capabilities

release-check:
    ./scripts/agent/release-check

release-script-test:
    ./scripts/agent/release-script-test

release-workflow-test:
    ./scripts/agent/release-workflow-test

npm-package version:
    ./scripts/agent/npm-package {{quote(version)}}

npm-package-check package_dir='.tmp/npm-package':
    ./scripts/agent/npm-package-check {{quote(package_dir)}}

npm-publish package_dir dist_tag:
    ./scripts/agent/npm-publish {{quote(package_dir)}} {{quote(dist_tag)}}
