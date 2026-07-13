set quiet := true
set no-cd := true

mod agent 'just/agent.just'
mod docs 'just/docs.just'

[doc('inspect, validate, tag, and publish a release; forward helper options after --')]
release *args:
    ./scripts/release/publish.sh {{args}}

[private]
default:
    @just --list --list-submodules
