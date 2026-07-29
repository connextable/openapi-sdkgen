set quiet
set no-cd

mod agent 'just/agent.just'
mod docs 'just/docs.just'

[private]
default:
    @just --list --list-submodules

[doc('inspect, validate, tag, and publish a release; forward helper options after --')]
release *args:
    ./scripts/release/publish.sh {{ args }}
