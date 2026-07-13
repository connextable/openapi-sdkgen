set quiet := true
set no-cd := true

mod agent 'just/agent.just'
mod docs 'just/docs.just'

[doc('validate, tag, and publish a release; accepts patch/minor/major or vX.Y.Z[-prerelease]')]
release version='':
    ./scripts/release/publish.sh {{quote(version)}}

[private]
default:
    @just --list --list-submodules
