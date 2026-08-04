## 1. Components

### CLI Client
lightweight application for submiting workflow and project definitions to a controller and requesting status updates.

#### Submit

#### Status

### Controller

Purpose: Owns orchestration.

Owner: One process.

Lifetime: From process startup until shutdown.

Responsibilities

- admit workflows
- compile work
- schedule work
- record results
- recover after restart

Does NOT Do

- execute Python
- execute R
- copy files directly
- perform customer computations

Collaborates With

Workers
Execution Environments
Persistence
Workflow Compiler

Questions

How should controller failover work?
Should scheduling be event driven?



### Worker

#### Claim

#### Execute

#### Report 

#### Repeat (not sure)

#### Heartbeat
### Execution Environment
An execution environment is a combination of Transport, Shell Dialect, Scheduler, and Runtime. 

#### SSH Transport

#### Docker Transport

#### Local Transport

#### Linux Bash Dialect

#### Windows Powershell Dialect

#### Direct Scheduler

#### Slurm Scheduler

#### Local Runtime

#### Singularity Runtime

#### Dockerize Slurm Test Environment

#### HPCC

## 2. Controller Objects

## 3. Worker Objects

## 4. Lifecycle of a Work item

## 5. Lifecycle of a Worker

## 6. Execution Environments

## 7. Persistance

## 8. Plugins

## 9. Failure and Recovery

## 10. Checkpointing

## 11. Log

2026-08-04

Today I started to document my own understanding of the GORC system on top the docs/concept which is very verbose.










