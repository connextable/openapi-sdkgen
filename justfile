set quiet := true
set no-cd := true

mod agent 'just/agent.just'
mod docs 'just/docs.just'

[doc('inspect, validate, tag, and publish a release; accepts release helper options')]
release version='':
    ./scripts/release/publish.sh {{quote(version)}}

[private]
default:
    @just --list --list-submodules
