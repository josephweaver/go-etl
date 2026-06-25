# SSH Transport Epic

Goal:
Allow controller Transport implementations to execute commands and copy files over SSH.

Non-goals:
- Full HPCC execution
- Persistent SSH session pooling
- Credential management UI

Slices:
- [ ] [001 Test fixture](001-test-strategy.md)
- [ ] [002 SSHTransport config](002-sshtransport-config.md)
- [ ] [003 SSHTransport connect](003-sshtransport-connect.md)
- [ ] [004 SSHTransport execute](004-sshtransport-execute.md)
- [ ] [005 SSHTransport copy](005-sshtransport-copy.md)
- [ ] [006 SSHTransport list](006-sshtransport-list.md)
- [ ] [007 SSHTransport filesystem commands](007-sshtransport-filesystem-commands.md)
- [ ] [008 Execution environment preflight and preparation](008-execution-environment-preflight.md)
- [ ] [009 SSHTransport retry and reconnect](009-sshtransport-retry-reconnect.md)
- [ ] [010 SSHTransport config wiring](010-sshtransport-config-wiring.md)
- [ ] [011 SSHTransport end-to-end integration](011-sshtransport-end-to-end-integration.md)
