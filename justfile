set quiet := true
set no-cd := true

mod agent 'just/agent.just'
mod docs 'just/docs.just'

[private]
default:
    @just --list --list-submodules
