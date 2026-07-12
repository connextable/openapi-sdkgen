set quiet := true
set no-cd := true

mod agent 'just/agent.just'

[private]
default:
    @just --list --list-submodules
